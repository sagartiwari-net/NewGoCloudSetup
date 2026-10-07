package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Cache research-health probes so dashboard HTML loads don't hammer Zik.
var (
	zikResearchHealthMu sync.Mutex
	zikResearchHealth   = map[int]struct {
		ok   bool
		at   time.Time
		detail string
	}{}
)

func zikAccountResearchHealthyCached(acc ToolAccount) (ok bool, detail string) {
	zikResearchHealthMu.Lock()
	if ent, hit := zikResearchHealth[acc.ID]; hit && time.Since(ent.at) < 90*time.Second {
		zikResearchHealthMu.Unlock()
		return ent.ok, ent.detail
	}
	zikResearchHealthMu.Unlock()

	_, researchOK, detail := zikProbeAccount(acc)
	// Network blips → fail-open (don't lock the whole tool). Auth 401 → fail-closed.
	if strings.Contains(detail, "_err=") {
		return true, "probe_network_skip: " + detail
	}
	zikResearchHealthMu.Lock()
	zikResearchHealth[acc.ID] = struct {
		ok     bool
		at     time.Time
		detail string
	}{ok: researchOK, at: time.Now(), detail: detail}
	zikResearchHealthMu.Unlock()
	return researchOK, detail
}

func zikInvalidateResearchHealth(accountID int) {
	zikResearchHealthMu.Lock()
	delete(zikResearchHealth, accountID)
	zikResearchHealthMu.Unlock()
}

// zikLooksLoggedOutBody — Zik research APIs when the panel cookie is dead.
func zikLooksLoggedOutBody(body []byte) bool {
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "please log in again") ||
		strings.Contains(lower, "you must be authenticated") ||
		strings.Contains(lower, "unauthorized")
}

func zikJWTClaimsSummary(cookieRaw string) string {
	bearer := zikAuthBearerFromAccount(cookieRaw)
	if bearer == "" {
		return "jwt=missing"
	}
	parts := strings.Split(bearer, ".")
	if len(parts) < 2 {
		return "jwt=malformed"
	}
	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}
	raw, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return "jwt=decode_fail"
		}
	}
	var claims map[string]interface{}
	if json.Unmarshal(raw, &claims) != nil {
		return "jwt=bad_json"
	}
	get := func(k string) string {
		if v, ok := claims[k]; ok && v != nil {
			return fmt.Sprint(v)
		}
		return ""
	}
	exp := get("exp")
	expired := zikJWTExpired(bearer)
	return fmt.Sprintf("pkg=%s active=%s email=%s exp=%s expired=%v",
		get("package"), get("active"), get("email"), exp, expired)
}

func zikUpstreamGET(acc ToolAccount, apiPath string) (status int, body []byte, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, proxyContextKey, acc.Proxy)
	url := "https://api.zikanalytics.com" + apiPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	cookieStr, _ := parseCookiesAndStorage(acc.Cookie)
	if cookieStr != "" {
		req.Header.Set("Cookie", cookieStr)
	}
	ua := acc.UserAgent
	if ua == "" {
		ua = loadConfig().UserAgent
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	if bearer := zikAuthBearerFromAccount(acc.Cookie); bearer != "" && !zikJWTExpired(bearer) {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	req.Header.Set("Origin", "https://app.zikanalytics.com")
	req.Header.Set("Referer", "https://app.zikanalytics.com/dashboard")
	req.Header.Set("Accept", "application/json")
	req.Host = "api.zikanalytics.com"

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, body, nil
}

// zikProbeAccount checks basic (GetStore) + research (suggestions) against live Zik API.
func zikProbeAccount(acc ToolAccount) (basicOK, researchOK bool, detail string) {
	claims := zikJWTClaimsSummary(acc.Cookie)
	st1, body1, err1 := zikUpstreamGET(acc, "/User/GetStore")
	if err1 != nil {
		return false, false, fmt.Sprintf("%s getStore_err=%v", claims, err1)
	}
	basicOK = st1 >= 200 && st1 < 300 && !zikLooksLoggedOutBody(body1)

	st2, body2, err2 := zikUpstreamGET(acc, "/competitiorResearch/suggestions?type=1")
	if err2 != nil {
		return basicOK, false, fmt.Sprintf("%s getStore=%d research_err=%v", claims, st1, err2)
	}
	researchOK = st2 >= 200 && st2 < 300 && !zikLooksLoggedOutBody(body2)
	detail = fmt.Sprintf("%s getStore=%d research=%d researchBody=%s",
		claims, st1, st2, truncateForLog(string(body2), 120))
	return basicOK, researchOK, detail
}

func listPanelActiveAccounts(cfg Config) ([]ToolAccount, error) {
	panelPickMu.Lock()
	defer panelPickMu.Unlock()
	db, err := openPanelDB(cfg)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(panelAccountSelect+`
		WHERE w.domain = ? AND a.status = 'active' AND a.cookie != ''
		`+panelAccountOrder, cfg.PublicHost)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ToolAccount
	for rows.Next() {
		var acc ToolAccount
		var showLimit int
		if err := rows.Scan(&acc.ID, &acc.Name, &acc.Cookie, &acc.UserAgent, &acc.Proxy, &showLimit); err != nil {
			continue
		}
		acc.ShowLimit = showLimit == 1
		acc.Cookie = strings.TrimSpace(acc.Cookie)
		if acc.Cookie == "" {
			continue
		}
		out = append(out, acc)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no active accounts")
	}
	return out, nil
}

// claimHealthyZikAccount probes Zik research API and returns the first account
// that can actually use competitor/product research (not just soft dashboard widgets).
func claimHealthyZikAccount(cfg Config) (ToolAccount, error) {
	accounts, err := listPanelActiveAccounts(cfg)
	if err != nil {
		return ToolAccount{}, err
	}
	var softDead []string
	netFails := 0
	for _, acc := range accounts {
		basicOK, researchOK, detail := zikProbeAccount(acc)
		log.Printf("[ZIK_PROBE] account=%s id=%d basic=%v research=%v %s proxy=%v",
			acc.Name, acc.ID, basicOK, researchOK, detail, strings.TrimSpace(acc.Proxy) != "")
		if researchOK {
			panelPickMu.Lock()
			if db, dbErr := openPanelDB(cfg); dbErr == nil {
				now := time.Now().UTC().Format(time.RFC3339)
				_, _ = db.Exec(`UPDATE accounts SET last_used_at=? WHERE id=?`, now, acc.ID)
			}
			panelPickMu.Unlock()
			log.Printf("[LB] claimed healthy Zik account '%s' (ID:%d) for %s", acc.Name, acc.ID, cfg.PublicHost)
			return acc, nil
		}
		if strings.Contains(detail, "_err=") {
			netFails++
			continue
		}
		if basicOK && !researchOK {
			softDead = append(softDead, fmt.Sprintf("%s(ID:%d)", acc.Name, acc.ID))
		}
	}
	// Transient upstream blips — don't lock users out; fall back to LRU claim.
	if netFails == len(accounts) && len(accounts) > 0 {
		log.Printf("[ZIK_PROBE] all probes network-failed — falling back to LRU claim")
		return claimPanelAccount(cfg)
	}
	if len(softDead) > 0 {
		return ToolAccount{}, fmt.Errorf("zik session dead for research on: %s — refresh cookies in panel (dashboard widgets may still look OK)", strings.Join(softDead, ", "))
	}
	return ToolAccount{}, fmt.Errorf("all Zik accounts failed auth probe — refresh cookies in panel")
}
