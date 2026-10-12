package main

import (
	"database/sql"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const uberGuestWatchScript = `<script>
(function () {
  function guest() {
    var t = (document.body && document.body.innerText || "").toLowerCase();
    if (t.indexOf("sign in") === -1) return false;
    return t.indexOf("unlock free credits") !== -1 || t.indexOf("free trial available") !== -1 || t.indexOf("start a 7-day trial") !== -1;
  }
  var tries = 0;
  var timer = setInterval(function () {
    tries += 1;
    if (tries > 24) { clearInterval(timer); return; }
    if (!guest()) return;
    clearInterval(timer);
    var n = 0;
    try { n = parseInt(sessionStorage.getItem("tm_acct_try") || "0", 10) || 0; } catch (e) {}
    if (n >= 3) { location.replace("/tm-unavailable"); return; }
    try { sessionStorage.setItem("tm_acct_back", location.pathname + location.search); } catch (e) {}
    location.replace("/tm-switching");
  }, 700);
})();
</script>`

const uberSwitchRetryScript = `<script>
(function () {
  var n = 0;
  try { n = parseInt(sessionStorage.getItem("tm_acct_try") || "0", 10) || 0; } catch (e) {}
  n += 1;
  try { sessionStorage.setItem("tm_acct_try", String(n)); } catch (e) {}
  if (n >= 3) {
    var title = document.querySelector("h1");
    var msg = document.querySelector(".msg");
    if (title) title.textContent = "Temporarily Unavailable";
    if (msg) msg.textContent = "Ubersuggest is temporarily unavailable. Please try again in a few minutes.";
    var pill = document.querySelector(".pill");
    if (pill) pill.style.display = "none";
    var foot = document.querySelector(".foot");
    if (foot) foot.textContent = "Open this tool again from your access link";
    var ring = document.querySelector(".ring");
    if (ring) ring.classList.remove("spin");
    try { sessionStorage.removeItem("tm_acct_try"); } catch (e) {}
    return;
  }
  var back = "/en/analyze/dashboard";
  try {
    var saved = sessionStorage.getItem("tm_acct_back") || "";
    if (saved.charAt(0) === "/" && saved.indexOf("//") !== 0 && saved.indexOf("/tm-") !== 0) back = saved;
  } catch (e) {}
  setTimeout(function () { window.location.replace(back); }, 1200);
})();
</script>`

func renderUberSwitchPage(w http.ResponseWriter) {
	writeLightCard(w, http.StatusOK, lightCard{
		Title:       "Switching account",
		Heading:     "Switching account",
		Message:     "Account logged out. Switching to another account...",
		Badge:       "Trying one account at a time",
		Footer:      "This keeps trying until an account is available",
		Spin:        true,
		ExtraScript: uberSwitchRetryScript,
	})
}

func renderUberUnavailable(w http.ResponseWriter) {
	writeLightCard(w, http.StatusServiceUnavailable, lightCard{
		Title:   "Temporarily Unavailable",
		Heading: "Temporarily Unavailable",
		Message: "Ubersuggest is temporarily unavailable. Please try again in a few minutes.",
		Footer:  "Open this tool again from your access link",
	})
}

func serveUberAccountSwitch(w http.ResponseWriter, r *http.Request, cfg Config, acc ToolAccount) {
	token := ""
	if c, err := r.Cookie("ct_session"); err == nil {
		token = c.Value
	}
	user, _ := panelSessionUsername(r)
	log.Printf("[FAILOVER] Ubersuggest logged out user=%s account=%s (ID:%d)", user, acc.Name, acc.ID)
	next, err := panelSwitchAccount(cfg, token, acc.ID, acc.Name, user, "guest_sign_in")
	if err != nil || next.ID == 0 || next.ID == acc.ID {
		log.Printf("[FAILOVER] no other Ubersuggest account user=%s err=%v", user, err)
		renderUberUnavailable(w)
		return
	}
	renderUberSwitchPage(w)
}

var (
	uberSwitchMu sync.Mutex
	uberSwitchAt = map[int]time.Time{}
)

func uberShouldRecordSwitch(accountID int) bool {
	if accountID <= 0 {
		return true
	}
	uberSwitchMu.Lock()
	defer uberSwitchMu.Unlock()
	if last, ok := uberSwitchAt[accountID]; ok && time.Since(last) < 45*time.Second {
		return false
	}
	uberSwitchAt[accountID] = time.Now()
	return true
}

func uberRecordSwitch(db *sql.DB, websiteID, accountID int, username, fromName, nextName, reason, clientIP string) {
	if db == nil || websiteID <= 0 || fromName == "" || !uberShouldRecordSwitch(accountID) {
		return
	}
	reason = strings.TrimSpace(reason)
	if len(reason) > 160 {
		reason = reason[:160]
	}
	if strings.TrimSpace(nextName) == "" {
		nextName = "(none)"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO switch_events (website_id, username, from_account_name, to_account_name, reason, switched_at) VALUES (?,?,?,?,?,?)`,
		websiteID, username, fromName, nextName, reason, now); err != nil {
		log.Printf("[LB] switch_events insert failed: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO logout_events (website_id, username, account_name, next_account_name, reason, client_ip, created_at) VALUES (?,?,?,?,?,?,?)`,
		websiteID, username, fromName, nextName, reason, clientIP, now); err != nil {
		log.Printf("[LB] logout_events insert failed: %v", err)
	}
}
