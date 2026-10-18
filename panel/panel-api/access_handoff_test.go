package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestAccessHandoff(t *testing.T) {
	t.Setenv("GT4RENTS_MYSQL_PASSWORD", "")
	t.Setenv("TOOL_PUBLIC_SCHEME", "https")
	db := openDB(filepath.Join(t.TempDir(), "panel.db"))
	t.Cleanup(func() { _ = db.Close() })
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO admin_sessions (token, operator_id, expires_at) VALUES ('handoff-master', 1, ?)`, expires); err != nil {
		t.Fatal(err)
	}
	var websiteID int
	var name, domain string
	if err := db.QueryRow(`SELECT id, name, domain FROM websites ORDER BY id LIMIT 1`).Scan(&websiteID, &name, &domain); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE websites SET access_slug='refs' WHERE id=?`, websiteID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO products (website_id, product_name, product_ids_json) VALUES (?,?,?)`, websiteID, "Shared", `["12"]`); err != nil {
		t.Fatal(err)
	}
	reseller, err := db.Exec(`INSERT INTO operators (username, password_hash, role, status) VALUES ('handoff-seller', 'x', 'reseller', 'active')`)
	if err != nil {
		t.Fatal(err)
	}
	resellerID, _ := reseller.LastInsertId()
	var otherID int
	if err := db.QueryRow(`SELECT id FROM websites WHERE id<>? ORDER BY id LIMIT 1`, websiteID).Scan(&otherID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO operator_websites (operator_id, website_id) VALUES (?,?)`, resellerID, otherID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO admin_sessions (token, operator_id, expires_at) VALUES ('handoff-reseller', ?, ?)`, resellerID, expires); err != nil {
		t.Fatal(err)
	}

	s := &server{db: db}
	mux := http.NewServeMux()
	s.routes(mux)

	if _, err := db.Exec(`INSERT INTO access_rejects (website_id, username, client_ip, reason, message, site_domain, tool_slug, created_at) VALUES (0,'ada','1.1.1.1','not_whitelisted','You are using from UnAuthorised website','toolsmandi.com','refs',?)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	created := callAs(t, mux, "handoff-master", http.MethodPost, "/api/access-sites", fmt.Sprintf(`{"domain":"https://toolsmandi.com/member","note":"main","website_ids":[%d]}`, websiteID))
	if created.Code != 200 {
		t.Fatalf("create site %d %s", created.Code, created.Body.String())
	}
	var site struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &site); err != nil || site.Key == "" {
		t.Fatalf("key missing: %s", created.Body.String())
	}
	var stale int
	if err := db.QueryRow(`SELECT COUNT(*) FROM access_rejects WHERE site_domain='toolsmandi.com' AND reason='not_whitelisted'`).Scan(&stale); err != nil {
		t.Fatal(err)
	}
	if stale != 0 {
		t.Fatal("allowing the site left the old reject")
	}
	listed := callAs(t, mux, "handoff-master", http.MethodGet, "/api/access-sites", "")
	if bytes.Contains(listed.Body.Bytes(), []byte(site.Key)) {
		t.Fatal("site list returned the key")
	}
	denied := callAs(t, mux, "handoff-reseller", http.MethodGet, "/api/access-sites", "")
	if denied.Code != 403 {
		t.Fatalf("reseller list status %d", denied.Code)
	}

	miss := postHandoff(t, mux, site.Key, "other.example", "refs", "ada", []string{"12"}, "1.2.3.4")
	if miss != msgUnauthorisedSite {
		t.Fatalf("whitelist miss %q", miss)
	}
	bad := postHandoff(t, mux, "not-the-key", "toolsmandi.com", "refs", "ada", []string{"12"}, "1.2.3.4")
	if bad != msgDirectAccess {
		t.Fatalf("bad signature %q", bad)
	}
	noPlan := postHandoff(t, mux, site.Key, "toolsmandi.com", "refs", "ada", []string{"3", "15"}, "1.2.3.4")
	if noPlan != "You do not have access of "+name {
		t.Fatalf("plan miss %q", noPlan)
	}
	okURL := postHandoffURL(t, mux, site.Key, "toolsmandi.com", "refs", "ada", []string{"3", "12", "32"}, "203.0.113.8")
	if okURL == "" || !bytes.Contains([]byte(okURL), []byte("https://"+domain+"/access?token=")) {
		t.Fatalf("open url %q", okURL)
	}
	var otherName string
	if err := db.QueryRow(`SELECT name FROM websites WHERE id=?`, otherID).Scan(&otherName); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE websites SET access_slug='other' WHERE id=?`, otherID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO products (website_id, product_name, product_ids_json) VALUES (?,?,?)`, otherID, "Other", `["12"]`); err != nil {
		t.Fatal(err)
	}
	blockedTool := postHandoff(t, mux, site.Key, "toolsmandi.com", "other", "ada", []string{"12"}, "198.51.100.4")
	if blockedTool != msgToolNotOnSite+otherName {
		t.Fatalf("tool not on site %q", blockedTool)
	}
	var tokenExpires string
	if err := db.QueryRow(`SELECT expires_at FROM access_tokens WHERE username='ada'`).Scan(&tokenExpires); err != nil {
		t.Fatal(err)
	}
	exp, err := time.Parse(time.RFC3339, tokenExpires)
	if err != nil {
		t.Fatal(err)
	}
	if exp.After(time.Now().Add(35*time.Second)) || exp.Before(time.Now()) {
		t.Fatalf("token expiry %s", tokenExpires)
	}
	if _, err := db.Exec(`DELETE FROM access_tokens WHERE username='ada'`); err != nil {
		t.Fatal(err)
	}
	var left int
	_ = db.QueryRow(`SELECT COUNT(*) FROM access_tokens WHERE username='ada'`).Scan(&left)
	if left != 0 {
		t.Fatal("token still usable")
	}

	var own, foreign int
	_ = db.QueryRow(`SELECT COUNT(*) FROM access_rejects WHERE website_id=?`, otherID).Scan(&own)
	_ = db.QueryRow(`SELECT COUNT(*) FROM access_rejects WHERE website_id=?`, websiteID).Scan(&foreign)
	if _, err := db.Exec(`INSERT INTO access_rejects (website_id, username, client_ip, reason, message, site_domain, tool_slug, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		otherID, "ada", "1.1.1.1", "no_plan", "own", "toolsmandi.com", "refs", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	callAs(t, mux, "handoff-reseller", http.MethodDelete, "/api/access-rejects", "")
	_ = db.QueryRow(`SELECT COUNT(*) FROM access_rejects WHERE website_id=?`, otherID).Scan(&own)
	_ = db.QueryRow(`SELECT COUNT(*) FROM access_rejects WHERE website_id=?`, websiteID).Scan(&foreign)
	if own != 0 {
		t.Fatal("reseller clear left their own rows")
	}
	if foreign == 0 {
		t.Fatal("reseller clear removed another website")
	}
}

func postHandoff(t *testing.T, handler http.Handler, key, site, tool, username string, ids []string, ip string) string {
	t.Helper()
	body := handoffBody(key, site, tool, username, ids, ip)
	req := httptest.NewRequest(http.MethodPost, "/api/access/handoff", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.9:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var out struct {
		Message string `json:"message"`
		OpenURL string `json:"open_url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(rec.Body.String())
	}
	if out.OpenURL != "" {
		t.Fatalf("expected a message, got %s", out.OpenURL)
	}
	return out.Message
}

func postHandoffURL(t *testing.T, handler http.Handler, key, site, tool, username string, ids []string, ip string) string {
	t.Helper()
	body := handoffBody(key, site, tool, username, ids, ip)
	req := httptest.NewRequest(http.MethodPost, "/api/access/handoff", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.9:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var out struct {
		OpenURL string `json:"open_url"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(rec.Body.String())
	}
	if out.Message != "" {
		t.Fatalf("handoff message %s", out.Message)
	}
	return out.OpenURL
}

func handoffBody(key, site, tool, username string, ids []string, ip string) []byte {
	ts := time.Now().Unix()
	nonce := username + "-" + ip + "-" + strconv.FormatInt(ts, 10) + "-" + tool + "-" + site
	canonical := accessCanonical(normalizeSiteDomain(site), normalizeAccessSlug(tool), username, ip, ids, ts, nonce)
	payload := map[string]any{
		"site": site, "tool": tool, "username": username, "product_ids": ids,
		"client_ip": ip, "timestamp": ts, "nonce": nonce, "signature": accessSign(key, canonical),
	}
	raw, _ := json.Marshal(payload)
	return raw
}

func TestFillButtonNames(t *testing.T) {
	t.Setenv("GT4RENTS_MYSQL_PASSWORD", "")
	db := openDB(filepath.Join(t.TempDir(), "panel.db"))
	t.Cleanup(func() { _ = db.Close() })
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO admin_sessions (token, operator_id, expires_at) VALUES ('slug-master', 1, ?)`, expires); err != nil {
		t.Fatal(err)
	}
	var ids []int
	rows, err := db.Query(`SELECT id FROM websites ORDER BY id LIMIT 3`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if len(ids) < 3 {
		t.Fatal("need three websites")
	}
	if _, err := db.Exec(`UPDATE websites SET domain='refs.gt4rents.com', access_slug='' WHERE id=?`, ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE websites SET domain='refs.example.com', access_slug='keep' WHERE id=?`, ids[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE websites SET domain='refs.other.com', access_slug='' WHERE id=?`, ids[2]); err != nil {
		t.Fatal(err)
	}
	s := &server{db: db}
	mux := http.NewServeMux()
	s.routes(mux)
	rec := callAs(t, mux, "slug-master", http.MethodPost, "/api/websites/button-names", fmt.Sprintf(`{"website_ids":[%d,%d,%d]}`, ids[0], ids[1], ids[2]))
	if rec.Code != 200 {
		t.Fatalf("fill %d %s", rec.Code, rec.Body.String())
	}
	var kept, first, second string
	if err := db.QueryRow(`SELECT access_slug FROM websites WHERE id=?`, ids[1]).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != "keep" {
		t.Fatalf("existing name changed to %s", kept)
	}
	if err := db.QueryRow(`SELECT access_slug FROM websites WHERE id=?`, ids[0]).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT access_slug FROM websites WHERE id=?`, ids[2]).Scan(&second); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{first: true, second: true}
	if !got["refs"] || !got["refs-2"] {
		t.Fatalf("names %s %s", first, second)
	}
}

func TestT4ToolzAhrefs(t *testing.T) {
	t.Setenv("GT4RENTS_MYSQL_PASSWORD", "")
	db := openDB(filepath.Join(t.TempDir(), "panel.db"))
	t.Cleanup(func() { _ = db.Close() })
	var id int
	var slug string
	if err := db.QueryRow(`SELECT id, access_slug FROM websites WHERE domain=?`, "ah2.t4toolz.com").Scan(&id, &slug); err != nil {
		t.Fatal(err)
	}
	if slug != "t4refs" || id <= 0 {
		t.Fatalf("website %d slug %s", id, slug)
	}
	if _, err := db.Exec(`UPDATE websites SET access_slug='ah2' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	ensureT4ToolzAhrefs(db)
	if err := db.QueryRow(`SELECT access_slug FROM websites WHERE id=?`, id).Scan(&slug); err != nil {
		t.Fatal(err)
	}
	if slug != "t4refs" {
		t.Fatalf("slug stayed %s", slug)
	}
}

func callAs(t *testing.T, handler http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
