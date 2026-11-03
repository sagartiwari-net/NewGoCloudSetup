package main

import (
	"bytes"
	"log"
	"strings"
	"sync"
	"time"
)

// Tracks the last successful credit charge so a failover account-switch can
// refund only what was just billed — never invent credits on dashboard/login hops.
type ahrefsRecentCredit struct {
	Amount         int
	Family         string
	Action         string
	Target         string
	LegacyEndpoint string
	At             time.Time
}

var (
	ahrefsRecentCreditMu sync.Mutex
	ahrefsRecentCredits  = map[string]ahrefsRecentCredit{}
)

const ahrefsCreditRevertWindow = 45 * time.Second

func ahrefsLooksLikeCSV(contentType string, body []byte) bool {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "text/csv") || strings.Contains(ct, "application/csv") ||
		strings.Contains(ct, "application/vnd.ms-excel") {
		return len(body) > 0
	}
	if len(body) >= 3 && body[0] == 0xEF && body[1] == 0xBB && body[2] == 0xBF {
		return true
	}
	if len(body) >= 2 && ((body[0] == 0xFF && body[1] == 0xFE) || (body[0] == 0xFE && body[1] == 0xFF)) {
		return true
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}
	// JSON / HTML export orchestration must not be billed as rows.
	if trimmed[0] == '{' || trimmed[0] == '[' || trimmed[0] == '<' {
		return false
	}
	sample := trimmed
	if len(sample) > 512 {
		sample = sample[:512]
	}
	if !bytes.Contains(sample, []byte(",")) && !bytes.Contains(sample, []byte("\t")) {
		return false
	}
	return bytes.Contains(sample, []byte("\n")) || bytes.Contains(sample, []byte{0x0a, 0x00})
}

// ahrefsExportBillAmount bills one successful CSV download.
// Prefer the requested size when the file is real CSV; never bill JSON "Export" APIs.
// If row parsing comes out ~2× the ask (known double-count symptom), keep the ask.
func ahrefsExportBillAmount(ask int, contentType string, body []byte) (amount int, ok bool) {
	if !ahrefsLooksLikeCSV(contentType, body) {
		return 0, false
	}
	rows := ahrefsCSVRows(body)
	if rows < 1 && ask < 1 {
		return 0, false
	}
	if ask > 0 {
		if rows >= ask*2 && rows <= ask*2+5 {
			return ask, true
		}
		if rows > 0 && rows < ask {
			return rows, true
		}
		return ask, true
	}
	return rows, rows > 0
}

func ahrefsNoteCreditCharge(username string, websiteID int, amount int, family, action, target, legacyEndpoint string) {
	if username == "" || amount < 1 {
		return
	}
	key := creditUserKey(username, websiteID)
	ahrefsRecentCreditMu.Lock()
	ahrefsRecentCredits[key] = ahrefsRecentCredit{
		Amount:         amount,
		Family:         family,
		Action:         action,
		Target:         target,
		LegacyEndpoint: legacyEndpoint,
		At:             time.Now(),
	}
	ahrefsRecentCreditMu.Unlock()
}

func ahrefsNoteLegacyCreditLog(username string, websiteID int, endpoint string) {
	if username == "" || endpoint == "" {
		return
	}
	key := creditUserKey(username, websiteID)
	ahrefsRecentCreditMu.Lock()
	defer ahrefsRecentCreditMu.Unlock()
	cur, ok := ahrefsRecentCredits[key]
	now := time.Now()
	if !ok || now.Sub(cur.At) > ahrefsCreditRevertWindow {
		ahrefsRecentCredits[key] = ahrefsRecentCredit{
			Amount:         1,
			LegacyEndpoint: endpoint,
			At:             now,
		}
		return
	}
	cur.LegacyEndpoint = endpoint
	if cur.Amount < 1 {
		cur.Amount = 1
	}
	cur.At = now
	ahrefsRecentCredits[key] = cur
}

// ahrefsRevertRecentCreditOnSwitch refunds the last credit only when one was
// charged recently for this user. No prior charge → no-op (dashboard/login safe).
func ahrefsRevertRecentCreditOnSwitch(username string) {
	if username == "" {
		return
	}
	websiteID := currentWebsiteID
	if dbPanel, err := openAhrefsPanel(); err == nil {
		if id := ahrefsWebsiteIDForUser(dbPanel, username, loadConfig().PublicHost); id > 0 {
			websiteID = id
		}
	}
	key := creditUserKey(username, websiteID)

	ahrefsRecentCreditMu.Lock()
	charge, ok := ahrefsRecentCredits[key]
	if ok {
		delete(ahrefsRecentCredits, key)
	}
	ahrefsRecentCreditMu.Unlock()
	if !ok || charge.Amount < 1 || time.Since(charge.At) > ahrefsCreditRevertWindow {
		return
	}

	revertedMeter := ahrefsRefundMeterCredits(username, websiteID, charge.Amount)
	revertedLegacy := ahrefsRefundLegacyCreditLog(username, websiteID, charge.LegacyEndpoint)
	if revertedMeter || revertedLegacy {
		log.Printf("[LIMITS] switch refund user=%s amount=%d family=%s action=%s meter=%v legacy=%v",
			username, charge.Amount, charge.Family, charge.Action, revertedMeter, revertedLegacy)
	}
}

func ahrefsRefundMeterCredits(username string, websiteID, amount int) bool {
	if amount < 1 {
		return false
	}
	dbPanel, err := openAhrefsPanel()
	if err != nil {
		return false
	}
	var userID int
	if err := dbPanel.QueryRow(`SELECT id FROM panel_users WHERE website_id=? AND username=?`, websiteID, username).Scan(&userID); err != nil || userID == 0 {
		return false
	}
	res, err := dbPanel.Exec(`UPDATE user_meters SET used = CASE WHEN used >= ? THEN used - ? ELSE 0 END WHERE user_id=? AND "key"='credits'`, amount, amount, userID)
	if err != nil {
		log.Printf("[LIMITS] switch meter refund failed user=%s: %v", username, err)
		return false
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false
	}
	_, _ = dbPanel.Exec(
		`INSERT INTO usage_events (website_id, username, limit_key, limit_label, reset_days, action, target_path, amount, timestamp)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		websiteID, username, "credits", "Credits", 1, "Account switch refund", "failover", -amount, time.Now().UTC().Format(time.RFC3339),
	)
	return true
}

func ahrefsRefundLegacyCreditLog(username string, websiteID int, endpoint string) bool {
	if db == nil {
		return false
	}
	var id int64
	var err error
	if strings.TrimSpace(endpoint) != "" {
		err = db.QueryRow(
			`SELECT id FROM ahrefs_credit_logs
			 WHERE username = ? AND website_id = ? AND endpoint = ?
			   AND timestamp >= DATE_SUB(NOW(), INTERVAL 45 SECOND)
			 ORDER BY id DESC LIMIT 1`,
			username, websiteID, endpoint,
		).Scan(&id)
	} else {
		err = db.QueryRow(
			`SELECT id FROM ahrefs_credit_logs
			 WHERE username = ? AND website_id = ?
			   AND timestamp >= DATE_SUB(NOW(), INTERVAL 45 SECOND)
			 ORDER BY id DESC LIMIT 1`,
			username, websiteID,
		).Scan(&id)
	}
	if err != nil || id == 0 {
		return false
	}
	res, err := db.Exec(`DELETE FROM ahrefs_credit_logs WHERE id = ?`, id)
	if err != nil {
		log.Printf("[LIMITS] switch legacy credit refund failed user=%s id=%d: %v", username, id, err)
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

func ahrefsExportErrorRefreshScript() string {
	return `<script data-tm-ahrefs-export-refresh="1">
(function(){
  if (window.__tmAhrefsExportRefresh) return;
  window.__tmAhrefsExportRefresh = true;
  var armed = true;
  function shouldReload(text){
    if (!text) return false;
    return text.indexOf('Something went wrong') !== -1 && text.indexOf('Request id:') !== -1;
  }
  function check(){
    if (!armed || !document.body) return;
    try {
      if (shouldReload(document.body.innerText || '')) {
        armed = false;
        setTimeout(function(){ location.reload(); }, 700);
      }
    } catch (e) {}
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', check);
  else check();
  try {
    var obs = new MutationObserver(check);
    obs.observe(document.documentElement, {childList:true, subtree:true, characterData:true});
  } catch (e) {}
})();
</script>`
}
