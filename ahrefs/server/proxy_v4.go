package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"ahrefs-bar-api/filecache"
)

type memCacheEntry struct {
	status  int
	body    []byte
	expires time.Time
}

type inflightWait struct {
	done   chan struct{}
	status int
	body   []byte
}

var (
	v4MemCache    = map[string]memCacheEntry{}
	v4MemCacheMu  sync.Mutex
	v4Inflight    = map[string]*inflightWait{}
	v4InflightMu  sync.Mutex
)

func wantsFresh(r *http.Request) bool {
	h := strings.TrimSpace(headerOr(r, "X-TM-Bar-Fresh"))
	if h == "1" || strings.EqualFold(h, "true") || strings.EqualFold(h, "yes") {
		return true
	}
	q := r.URL.Query().Get("fresh")
	return q == "1" || strings.EqualFold(q, "true")
}

func (s *server) handleV4(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	token := headerOr(r, "X-TM-Bar-Token", "Authorization")
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		writeAhrefsError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	sess, code, err := s.lookupSessionChecked(r, token)
	if err != nil {
		writeAhrefsError(w, http.StatusInternalServerError, "InternalError")
		return
	}
	if sess == nil {
		if code == "fingerprint_required" || code == "fingerprint_mismatch" {
			writeAhrefsJSON(w, http.StatusUnauthorized, []any{"Error", code})
			return
		}
		writeAhrefsError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	endpoint := strings.TrimPrefix(r.URL.Path, "/v4/")
	endpoint = strings.Trim(endpoint, "/")
	if i := strings.IndexByte(endpoint, '/'); i >= 0 {
		endpoint = endpoint[:i]
	}
	if endpoint == "" {
		writeAhrefsError(w, http.StatusNotFound, "NotFound")
		return
	}

	// Sanitized user info — never leak real Ahrefs workspace.
	if endpoint == "tbGetUserInfoV3" {
		writeAhrefsJSON(w, http.StatusOK, sanitizeUserInfo(sess))
		return
	}

	query := stripFreshParam(r.URL.RawQuery)
	targetHint := queryTarget(query)
	if isJunkAhrefsTarget(targetHint) || isPrivateOrLocalTarget(targetHint) {
		// Google SERP /goto|/url noise + localhost/LAN — don't burn upstream.
		_, _ = s.logUsage(sess, 0, endpoint, shortJunkTarget(targetHint), 0, false, 400)
		writeAhrefsJSON(w, http.StatusOK, []any{"Error", "InvalidTarget"})
		return
	}
	fresh := wantsFresh(r)
	// KeywordStats is billable (1 credit). Soft-nav / MutationObserver retries used to
	// re-hit upstream dozens of times per SERP and burn hourly meters → LimitReached
	// while domain metrics (cost 0) still looked fine. Cache + inflight dedupe first.
	cacheable := r.Method == http.MethodGet &&
		(endpoint == "tbGetIconV3" || endpoint == "tbGetHeaderV3" || endpoint == "tbGetKeywordStatsV3")
	kwCache := endpoint == "tbGetKeywordStatsV3"
	cacheTTL := 15 * time.Minute
	if kwCache {
		cacheTTL = 60 * time.Minute
	}
	cacheKey := ""
	var inflightLeader *inflightWait
	if cacheable {
		cacheKey = v4CacheKey(endpoint, query)
		if !fresh {
			if hit, ok := v4CacheGet(cacheKey); ok {
				_, _ = s.logUsage(sess, 0, endpoint, queryTarget(query), 0, true, hit.status)
				w.Header().Set("X-TM-Bar-Cache", "HIT")
				writeRaw(w, hit.status, hit.body)
				return
			}
			leader, follower := v4InflightAcquire(cacheKey)
			if follower != nil {
				<-follower.done
				_, _ = s.logUsage(sess, 0, endpoint, queryTarget(query), 0, true, follower.status)
				w.Header().Set("X-TM-Bar-Cache", "DEDUP")
				writeRaw(w, follower.status, follower.body)
				return
			}
			inflightLeader = leader
		} else {
			v4CacheDelete(cacheKey)
		}
	}
	defer func() {
		if inflightLeader != nil {
			v4InflightFinish(cacheKey, inflightLeader)
		}
	}()

	respond := func(status int, respBody []byte, cacheTag string) {
		if inflightLeader != nil {
			inflightLeader.status = status
			inflightLeader.body = append([]byte(nil), respBody...)
		}
		if cacheTag != "" {
			w.Header().Set("X-TM-Bar-Cache", cacheTag)
		}
		writeRaw(w, status, respBody)
	}

	// Durable shared file cache — serves even when meters are exhausted (no upstream).
	if !fresh && cacheable {
		if payload, ok := s.fileCacheGet(endpoint, query); ok {
			_, _ = s.logUsage(sess, 0, endpoint, queryTarget(query), 0, true, 200)
			if cacheKey != "" {
				v4CacheSet(cacheKey, 200, payload, cacheTTL)
			}
			respond(http.StatusOK, payload, "HIT_SERVER")
			return
		}
	}

	cost := s.endpointCost(endpoint)
	charged := false

	// Hard gate: never call official Ahrefs unless BOTH windows still have room
	// (hourly remaining > 0 AND daily remaining > 0). Applies to free Header/Url
	// too — otherwise URL metrics keep working after keyword hourly is exhausted.
	snap, err := s.meterSnapshot(sess.ResellerID, sess.AmemberLogin)
	if err != nil {
		writeAhrefsError(w, http.StatusInternalServerError, "InternalError")
		return
	}
	if snap.HourlyRemaining <= 0 || snap.DailyRemaining <= 0 {
		reason := "hourly"
		if snap.HourlyRemaining > 0 {
			reason = "daily"
		}
		s.writeLimitReached(w, respond, sess, endpoint, query, reason, snap)
		return
	}

	if cost > 0 {
		ok, chargedSnap, reason, err := s.chargeMeters(sess.ResellerID, sess.AmemberLogin, cost)
		if err != nil {
			writeAhrefsError(w, http.StatusInternalServerError, "InternalError")
			return
		}
		if !ok {
			s.writeLimitReached(w, respond, sess, endpoint, query, reason, chargedSnap)
			return
		}
		snap = chargedSnap
		charged = true
	}

	accounts, err := s.pickAccountOrder(sess)
	if err != nil {
		if charged {
			_ = s.refundMeters(sess.ResellerID, sess.AmemberLogin, cost)
		}
		writeAhrefsError(w, http.StatusInternalServerError, "InternalError")
		return
	}
	if len(accounts) == 0 {
		if charged {
			_ = s.refundMeters(sess.ResellerID, sess.AmemberLogin, cost)
		}
		s.alertPoolEmpty(sess.ResellerID)
		retrySec, retryAt, poolMsg := s.poolUnavailableDetail(sess.ResellerID)
		detail := map[string]any{
			"retry_after_sec": retrySec,
			"retry_at":        retryAt,
			"message":         poolMsg,
		}
		raw, _ := json.Marshal([]any{"Error", "DataSourceUnavailable", detail})
		if retrySec > 0 {
			w.Header().Set("X-TM-Bar-Retry-After", itoa(retrySec))
		}
		w.Header().Set("X-TM-Bar-Limit-Message", poolMsg)
		respond(http.StatusServiceUnavailable, raw, "BYPASS")
		return
	}

	body, _ := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	_ = r.Body.Close()

	stickyMins := s.stickyMinutes(sess.ResellerID)
	var lastStatus int
	var lastBody []byte
	var lastAccID int64
	cacheTag := "MISS"
	if fresh {
		cacheTag = "FRESH"
	}

	for i, acc := range accounts {
		s.touchAccountUsed(acc.ID)
		status, respBody := s.callUpstream(acc, endpoint, query, r.Method, body)
		lastStatus, lastBody, lastAccID = status, respBody, acc.ID

		if shouldFailover(status, respBody) {
			reason := "upstream_" + itoa(status)
			if isUpstreamAuthFail(status) {
				// Dead/revoked token — park so other pool accounts can take over.
				s.markAccountBad(acc.ID, reason)
			} else {
				// 429 / Ahrefs RateLimit / LimitReached on a still-valid token.
				// Soft-skip only — user hourly/daily meters remain the quota.
				noteAccountSoftSkip(acc.ID)
				s.noteAccountUpstream(acc.ID, reason+"_soft")
			}
			if acc.ID == sess.StickyAccountID.Int64 {
				s.clearSessionSticky(sess.Token)
			}
			if i+1 < len(accounts) {
				_, _ = s.logUsage(sess, acc.ID, endpoint, queryTarget(query), 0, false, status)
				continue
			}
			// Last account — fall through; single log after the loop.
			break
		}

		logCredits := 0
		if charged {
			logCredits = cost
		}
		_, _ = s.logUsage(sess, acc.ID, endpoint, queryTarget(query), logCredits, false, status)
		if charged && logCredits > 0 {
			s.checkUsageSpike(sess.ResellerID)
		}
		if status >= 500 && charged && s.refundOn5xx(sess.ResellerID) {
			_ = s.refundMeters(sess.ResellerID, sess.AmemberLogin, cost)
		}
		if status == 200 && looksLikeAhrefsOK(respBody) {
			s.setSessionSticky(sess.Token, acc.ID, stickyMins)
			if cacheable {
				v4CacheSet(cacheKey, status, respBody, cacheTTL)
			}
			s.fileCachePut(endpoint, query, respBody, "upstream")
		}
		respond(status, respBody, cacheTag)
		return
	}

	rateLimited := isUpstreamRateLimited(lastStatus, lastBody)
	if charged && (rateLimited || (lastStatus >= 500 && s.refundOn5xx(sess.ResellerID))) {
		_ = s.refundMeters(sess.ResellerID, sess.AmemberLogin, cost)
	}
	_, _ = s.logUsage(sess, lastAccID, endpoint, queryTarget(query), 0, false, lastStatus)
	if lastStatus == 0 {
		writeAhrefsError(w, http.StatusServiceUnavailable, "DataSourceUnavailable")
		return
	}
	// All pool accounts rate-limited — soft message, do not park the pool.
	if rateLimited {
		msg := "Ahrefs is busy right now. Try again in a few seconds."
		detail := map[string]any{
			"retry_after_sec": 15,
			"message":         msg,
		}
		raw, _ := json.Marshal([]any{"Error", "RateLimit", detail})
		w.Header().Set("X-TM-Bar-Retry-After", "15")
		w.Header().Set("X-TM-Bar-Limit-Message", msg)
		respond(http.StatusOK, raw, "BYPASS")
		return
	}
	if !looksLikeAhrefsToolbar(lastBody) && lastStatus >= 500 {
		writeAhrefsError(w, http.StatusServiceUnavailable, "DataSourceUnavailable")
		return
	}
	respond(lastStatus, lastBody, cacheTag)
}

// writeLimitReached returns Melange-compatible LimitReached and never touches upstream.
func (s *server) writeLimitReached(
	w http.ResponseWriter,
	respond func(int, []byte, string),
	sess *barSession,
	endpoint, query, reason string,
	snap meterSnapshot,
) {
	retrySec, _ := s.meterRetryAfter(reason, sess.ResellerID, sess.AmemberLogin)
	msg := meterLimitMessage(reason, retrySec)
	raw, _ := json.Marshal([]any{"Error", "LimitReached"})
	w.Header().Set("X-TM-Bar-Limit", reason)
	w.Header().Set("X-TM-Bar-Retry-After", itoa(retrySec))
	w.Header().Set("X-TM-Bar-Limit-Message", msg)
	w.Header().Set("X-TM-Bar-Limit-Window", reason)
	w.Header().Set("X-TM-Bar-Hourly", itoa(snap.HourlyUsed)+"/"+itoa(snap.HourlyLimit))
	w.Header().Set("X-TM-Bar-Daily", itoa(snap.DailyUsed)+"/"+itoa(snap.DailyLimit))
	_, _ = s.logUsage(sess, 0, endpoint, queryTarget(query), 0, false, 429)
	respond(http.StatusOK, raw, "BYPASS")
}

func (s *server) callUpstream(acc barAccount, endpoint, query, method string, body []byte) (int, []byte) {
	useMock := s.cfg.MockUpstream || acc.Bearer == "MOCK" || strings.HasPrefix(acc.Bearer, "MOCK:")
	if useMock {
		return mockUpstreamForBearer(acc.Bearer, endpoint, query, method, body)
	}
	// Ahrefs retired tbGetUrlV3 (404 HTML). Melange still calls it for the
	// SERP/icon DR chip — synthesize from Header which carries the same stats.
	if endpoint == "tbGetUrlV3" {
		status, urlBody, headerBody := synthesizeUrlFromHeader(acc.Bearer, query)
		// Store full Header compact (not icon-only) under the shared URL key.
		if status == 200 && looksLikeAhrefsOK(headerBody) {
			s.fileCachePut("tbGetHeaderV3", query, headerBody, "upstream")
		}
		return status, urlBody
	}
	target := "https://ahrefs.com/v4/" + endpoint
	if query != "" {
		target += "?" + query
	}
	return forwardAhrefs(target, method, body, acc.Bearer)
}

// synthesizeUrlFromHeader maps tbGetHeaderV3 → tbGetUrlV3 Ok shape.
// Returns (status, urlBody, headerBody). headerBody is the full Header Ok JSON when available.
func synthesizeUrlFromHeader(bearer, query string) (int, []byte, []byte) {
	headerURL := "https://ahrefs.com/v4/tbGetHeaderV3"
	if query != "" {
		headerURL += "?" + query
	}
	status, raw := forwardAhrefs(headerURL, http.MethodGet, nil, bearer)
	if status != 200 || !looksLikeAhrefsOK(raw) {
		return status, raw, nil
	}
	var parsed []any
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed) < 2 {
		return status, raw, nil
	}
	obj, ok := parsed[1].(map[string]any)
	if !ok {
		return status, raw, nil
	}
	domainRating, ahrefsRank, urlRating := 0.0, 0.0, 0.0
	if root, ok := obj["root_domain"].(map[string]any); ok {
		if st, ok := root["stats"].(map[string]any); ok {
			domainRating = anyFloat(st["domain_rating"])
			ahrefsRank = anyFloat(st["ahrefs_rank"])
		}
	}
	if page, ok := obj["page"].(map[string]any); ok {
		if st, ok := page["stats"].(map[string]any); ok {
			urlRating = anyFloat(st["url_rating"])
		}
	}
	out, _ := json.Marshal([]any{"Ok", map[string]any{
		"stats": map[string]any{
			"domain_rating": domainRating,
			"ahrefs_rank":   int(ahrefsRank),
			"url_rating":    int(urlRating),
		},
	}})
	return 200, out, raw
}

func anyFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

func shouldFailover(status int, body []byte) bool {
	return isUpstreamAuthFail(status) || isUpstreamRateLimited(status, body)
}

func isUpstreamAuthFail(status int) bool {
	return status == 401 || status == 403
}

func isUpstreamRateLimited(status int, body []byte) bool {
	if status == 429 {
		return true
	}
	trim := strings.TrimSpace(string(body))
	return strings.Contains(trim, `"LimitReached"`) || strings.Contains(trim, `"RateLimit"`)
}

func mockUpstreamForBearer(bearer, endpoint, query, method string, body []byte) (int, []byte) {
	b := strings.ToUpper(strings.TrimSpace(bearer))
	if strings.Contains(b, "MOCK:401") || strings.Contains(b, "MOCK:FAIL") {
		raw, _ := json.Marshal([]any{"Error", "Unauthorized"})
		return 401, raw
	}
	if strings.Contains(b, "MOCK:403") {
		raw, _ := json.Marshal([]any{"Error", "Forbidden"})
		return 403, raw
	}
	return mockUpstream(endpoint, query, method, body)
}

func looksLikeAhrefsToolbar(body []byte) bool {
	trim := strings.TrimSpace(string(body))
	return strings.HasPrefix(trim, "[")
}

func (s *server) endpointCost(endpoint string) int {
	var credits int
	var class string
	err := s.db.QueryRow(`SELECT credits, class FROM ahrefs_bar_endpoint_costs WHERE endpoint = ?`, endpoint).Scan(&credits, &class)
	if err != nil {
		return 1
	}
	return credits
}

func (s *server) logUsage(sess *barSession, accountID int64, endpoint, target string, credits int, cacheHit bool, status int) (int64, error) {
	ch := 0
	if cacheHit {
		ch = 1
	}
	res, err := s.db.Exec(`
INSERT INTO ahrefs_bar_usage_log
  (reseller_id, amember_login, account_id, endpoint, target, credits, cache_hit, status_code, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ResellerID, sess.AmemberLogin, accountID, endpoint, truncateStr(target, 500),
		credits, ch, status, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func sanitizeUserInfo(sess *barSession) any {
	// Official toolbar (event.js) decodes Melange variants:
	//   keywordsLimit / siteExplorerLimit: "Unlimited" | ["Limited", {limit, usage}]
	//   workspace: { name: string }
	// Then Fs() maps Unlimited → 0 (fetch metrics). Plain ints / workspaceName
	// fail decode → default siteExplorerLimit:2 (Starter) → Metrics never loads.
	email := sess.Email
	if email == "" {
		email = sess.AmemberLogin + "@toolsmandi.local"
	}
	return []any{
		"Ok",
		map[string]any{
			"email":             email,
			"keywordsLimit":     "Unlimited",
			"siteExplorerLimit": "Unlimited",
			"workspace": map[string]any{
				"name": "ToolsMandi",
			},
		},
	}
}

func mockUpstream(endpoint, query, method string, body []byte) (int, []byte) {
	switch endpoint {
	case "tbGetIconV3":
		payload := []any{
			"Ok",
			map[string]any{
				"stats": map[string]any{
					"domain_rating": 72.0,
					"ahrefs_rank":   120000,
					"url_rating":    45,
				},
			},
		}
		b, _ := json.Marshal(payload)
		return 200, b
	case "tbGetHeaderV3":
		payload := []any{"Ok", map[string]any{
			"page": map[string]any{"id": 1, "label": "PAGE", "stats": map[string]any{
				"url_rating": 45, "backlinks": 1500, "refdomains": 320, "traffic": 12000.0, "keywords": 890,
			}},
			"root_domain": map[string]any{"id": 2, "label": "ROOT DOMAIN", "stats": map[string]any{
				"domain_rating": 72.0, "ahrefs_rank": 120000, "backlinks": 1500, "refdomains": 320, "traffic": 12000.0, "keywords": 890,
			}},
		}}
		b, _ := json.Marshal(payload)
		return 200, b
	case "tbGetKeywordStatsV3":
		// Shape matches live Ahrefs toolbar: { stats: { volume|difficulty|cpc: { value } } }
		kw := queryTarget(query)
		vol := 1200
		if kw != "" {
			vol = 100 + (len(kw)*37)%9000
		}
		payload := []any{"Ok", map[string]any{
			"stats": map[string]any{
				"volume":     map[string]any{"value": vol},
				"difficulty": map[string]any{"value": 34},
				"cpc":        map[string]any{"value": 0.42},
				"traffic":    map[string]any{"value": 500},
			},
		}}
		b, _ := json.Marshal(payload)
		return 200, b
	default:
		payload := []any{"Ok", map[string]any{"mock": true, "endpoint": endpoint, "method": method}}
		b, _ := json.Marshal(payload)
		return 200, b
	}
}

var (
	ahrefsTransport = &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     30 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	ahrefsHTTP = &http.Client{Timeout: 30 * time.Second, Transport: ahrefsTransport}
)

func forwardAhrefs(targetURL, method string, body []byte, bearer string) (int, []byte) {
	status, raw := doForwardAhrefs(targetURL, method, body, bearer)
	// Cloudflare/Ahrefs sometimes pins 429 on a reused keep-alive connection
	// while a fresh TCP/TLS session still succeeds — drop idle conns + retry once.
	if status == 429 {
		ahrefsTransport.CloseIdleConnections()
		time.Sleep(350 * time.Millisecond)
		status, raw = doForwardAhrefs(targetURL, method, body, bearer)
	}
	return status, raw
}

func doForwardAhrefs(targetURL, method string, body []byte, bearer string) (int, []byte) {
	var reader io.Reader
	if method != http.MethodGet && len(body) > 0 {
		reader = strings.NewReader(string(body))
	}
	req, err := http.NewRequest(method, targetURL, reader)
	if err != nil {
		b, _ := json.Marshal([]any{"Error", "NetworkError"})
		return 502, b
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json;charset=utf-8")
	}
	req.Header.Set("Origin", "https://ahrefs.com")
	req.Header.Set("Referer", "https://ahrefs.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	resp, err := ahrefsHTTP.Do(req)
	if err != nil {
		b, _ := json.Marshal([]any{"Error", "NetworkError"})
		return 502, b
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return resp.StatusCode, raw
}

func writeAhrefsError(w http.ResponseWriter, status int, code string) {
	b, _ := json.Marshal([]any{"Error", code})
	writeRaw(w, status, b)
}

func writeAhrefsJSON(w http.ResponseWriter, status int, v any) {
	b, _ := json.Marshal(v)
	writeRaw(w, status, b)
}

func writeRaw(w http.ResponseWriter, status int, body []byte) {
	if status <= 0 {
		status = 502
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func looksLikeAhrefsOK(body []byte) bool {
	trim := strings.TrimSpace(string(body))
	return strings.HasPrefix(trim, "[") && strings.Contains(trim, `"Ok"`)
}

func queryTarget(query string) string {
	obj := queryInputObject(query)
	if t, ok := obj["target"].(string); ok {
		return t
	}
	if k, ok := obj["keyword"].(string); ok {
		return k
	}
	return ""
}

func queryInputObject(query string) map[string]any {
	for _, part := range strings.Split(query, "&") {
		if strings.HasPrefix(part, "input=") {
			raw := strings.TrimPrefix(part, "input=")
			raw, _ = urlQueryUnescape(raw)
			var obj map[string]any
			if json.Unmarshal([]byte(raw), &obj) == nil {
				return obj
			}
		}
	}
	return nil
}

func (s *server) fileCacheGet(endpoint, query string) ([]byte, bool) {
	if s.fileCache == nil || !s.fileCache.Enabled() {
		return nil, false
	}
	kind, key := fileCacheKey(endpoint, query)
	if key == "" {
		return nil, false
	}
	body, ok, err := s.fileCache.GetAhrefsBody(kind, key, endpoint)
	if err != nil || !ok {
		return nil, false
	}
	return body, true
}

func (s *server) fileCachePut(endpoint, query string, payload []byte, source string) {
	if s.fileCache == nil || !s.fileCache.Enabled() || !looksLikeAhrefsOK(payload) {
		return
	}
	// Icon-only Url body must not overwrite full Header compact under the same key.
	if endpoint == "tbGetUrlV3" {
		return
	}
	kind, key := fileCacheKey(endpoint, query)
	if key == "" {
		return
	}
	_ = s.fileCache.PutAhrefsBody(kind, key, payload, source)
}

func fileCacheKey(endpoint, query string) (filecache.Kind, string) {
	obj := queryInputObject(query)
	switch endpoint {
	case "tbGetHeaderV3", "tbGetUrlV3":
		t, _ := obj["target"].(string)
		t = filecache.NormalizeURL(t)
		if t == "" {
			return "", ""
		}
		return filecache.KindURL, t
	case "tbGetKeywordStatsV3":
		kw, _ := obj["keyword"].(string)
		co, _ := obj["country"].(string)
		kw, co = filecache.NormalizeKeyword(kw, co)
		if kw == "" {
			return "", ""
		}
		return filecache.KindKeyword, filecache.KeywordKey(kw, co)
	default:
		return "", ""
	}
}

// isJunkAhrefsTarget filters Google SERP redirect paths and other non-host targets
// that Ahrefs rejects as InvalidTarget (and that blow up usage logs).
func isPrivateOrLocalTarget(target string) bool {
	t := strings.TrimSpace(strings.ToLower(target))
	if t == "" {
		return false
	}
	t = strings.TrimPrefix(strings.TrimPrefix(t, "https://"), "http://")
	host := t
	if i := strings.IndexAny(host, "/:?"); i >= 0 {
		host = host[:i]
	}
	if host == "localhost" || host == "127.0.0.1" || host == "0.0.0.0" ||
		host == "::1" || host == "[::1]" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if strings.HasPrefix(host, "10.") || strings.HasPrefix(host, "192.168.") ||
		strings.HasPrefix(host, "169.254.") {
		return true
	}
	if strings.HasPrefix(host, "172.") {
		parts := strings.Split(host, ".")
		if len(parts) >= 2 {
			if n, err := strconv.Atoi(parts[1]); err == nil && n >= 16 && n <= 31 {
				return true
			}
		}
	}
	return false
}

func isJunkAhrefsTarget(target string) bool {
	t := strings.TrimSpace(target)
	if t == "" {
		return false // empty is ok for endpoints like starter price
	}
	low := strings.ToLower(t)
	if strings.HasPrefix(low, "/goto") || strings.HasPrefix(low, "/url") ||
		strings.HasPrefix(low, "/search") || strings.HasPrefix(low, "/imgres") {
		return true
	}
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		// Absolute Google redirect / search pages are not useful SERP targets.
		if strings.Contains(low, "google.") &&
			(strings.Contains(low, "/goto") || strings.Contains(low, "/url?") ||
				strings.Contains(low, "/search?") || strings.HasSuffix(low, "google.com/") ||
				strings.HasSuffix(low, "google.co.in/") || strings.HasSuffix(low, "google.com/index.html") ||
				strings.HasSuffix(low, "google.co.in/index.html")) {
			return true
		}
		return false
	}
	// Bare domains / hostnames are fine; other relative paths are not.
	if strings.HasPrefix(t, "/") {
		return true
	}
	return false
}

func shortJunkTarget(target string) string {
	t := strings.TrimSpace(target)
	if t == "" {
		return "invalid"
	}
	if len(t) > 48 {
		t = t[:48]
	}
	return "invalid:" + t
}

func urlQueryUnescape(s string) (string, error) {
	replacer := strings.NewReplacer("+", " ")
	s = replacer.Replace(s)
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '%' && i+2 < len(s) {
			var n byte
			ok := true
			for _, c := range []byte{s[i+1], s[i+2]} {
				n <<= 4
				switch {
				case c >= '0' && c <= '9':
					n |= c - '0'
				case c >= 'a' && c <= 'f':
					n |= c - 'a' + 10
				case c >= 'A' && c <= 'F':
					n |= c - 'A' + 10
				default:
					ok = false
				}
			}
			if ok {
				b.WriteByte(n)
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String(), nil
}

func v4CacheKey(endpoint, query string) string {
	sum := sha1.Sum([]byte(endpoint + "?" + query))
	return hex.EncodeToString(sum[:])
}

func v4CacheGet(key string) (memCacheEntry, bool) {
	v4MemCacheMu.Lock()
	defer v4MemCacheMu.Unlock()
	e, ok := v4MemCache[key]
	if !ok || time.Now().After(e.expires) {
		return memCacheEntry{}, false
	}
	return e, true
}

func v4CacheSet(key string, status int, body []byte, ttl time.Duration) {
	v4MemCacheMu.Lock()
	defer v4MemCacheMu.Unlock()
	cp := append([]byte(nil), body...)
	v4MemCache[key] = memCacheEntry{status: status, body: cp, expires: time.Now().Add(ttl)}
}

func v4CacheDelete(key string) {
	v4MemCacheMu.Lock()
	defer v4MemCacheMu.Unlock()
	delete(v4MemCache, key)
}

func stripFreshParam(rawQuery string) string {
	if rawQuery == "" || !strings.Contains(rawQuery, "fresh=") {
		return rawQuery
	}
	parts := strings.Split(rawQuery, "&")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.HasPrefix(p, "fresh=") {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, "&")
}

// v4InflightAcquire returns (leader, nil) for the first caller, or (nil, follower) for joiners.
func v4InflightAcquire(key string) (*inflightWait, *inflightWait) {
	v4InflightMu.Lock()
	defer v4InflightMu.Unlock()
	if existing, ok := v4Inflight[key]; ok {
		return nil, existing
	}
	w := &inflightWait{done: make(chan struct{})}
	v4Inflight[key] = w
	return w, nil
}

func v4InflightFinish(key string, leader *inflightWait) {
	if leader == nil {
		return
	}
	v4InflightMu.Lock()
	if cur, ok := v4Inflight[key]; ok && cur == leader {
		delete(v4Inflight, key)
	}
	v4InflightMu.Unlock()
	close(leader.done)
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
