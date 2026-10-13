package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestRecloudPageFlow(t *testing.T) {
	t.Setenv("GT4RENTS_MYSQL_PASSWORD", "")
	db := openRecloudTestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	var websiteID int
	var toolName string
	if err := db.QueryRow(`SELECT w.id, t.name FROM websites w JOIN tools t ON t.id=w.tool_id WHERE w.kind='recloud'`).Scan(&websiteID, &toolName); err != nil {
		t.Fatal(err)
	}
	var otherID int
	if err := db.QueryRow(`SELECT id FROM websites WHERE kind=''`).Scan(&otherID); err != nil {
		t.Fatal(err)
	}

	s := &server{db: db}
	mux := http.NewServeMux()
	s.routes(mux)

	list := callJSON(t, mux, http.MethodGet, "/api/recloud", "")
	if list.Code != 200 {
		t.Fatalf("list status %d", list.Code)
	}
	if bytes.Contains(list.Body.Bytes(), []byte("kept-cookie")) {
		t.Fatal("recloud list included a cookie")
	}
	var listed struct {
		Items []struct {
			WebsiteID   int    `json:"website_id"`
			ToolName    string `json:"tool_name"`
			UpstreamURL string `json:"upstream_url"`
		} `json:"items"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 1 || listed.Items[0].WebsiteID != websiteID || listed.Items[0].ToolName != toolName {
		t.Fatalf("list items = %d", len(listed.Items))
	}
	if listed.Items[0].UpstreamURL != "https://old.example" {
		t.Fatal("list did not return the saved upstream URL")
	}

	accounts := callJSON(t, mux, http.MethodGet, "/api/accounts?page=1&pageSize=50", "")
	var accountPage struct {
		Items []struct {
			WebsiteID int `json:"website_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(accounts.Body.Bytes(), &accountPage); err != nil {
		t.Fatal(err)
	}
	for _, item := range accountPage.Items {
		if item.WebsiteID == websiteID {
			t.Fatal("mapped accounts listed a recloud website")
		}
	}
	if len(accountPage.Items) != 1 || accountPage.Items[0].WebsiteID != otherID {
		t.Fatal("mapped accounts dropped a normal website")
	}

	dropped := callJSON(t, mux, http.MethodGet, "/api/websites/options?exclude=recloud", "")
	var options struct {
		Items []struct {
			ID int `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(dropped.Body.Bytes(), &options); err != nil {
		t.Fatal(err)
	}
	for _, item := range options.Items {
		if item.ID == websiteID {
			t.Fatal("mapped account website dropdown listed a recloud website")
		}
	}
	full := callJSON(t, mux, http.MethodGet, "/api/websites/options", "")
	var all struct {
		Items []struct {
			ID int `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(full.Body.Bytes(), &all); err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, item := range all.Items {
		if item.ID == websiteID {
			seen = true
		}
	}
	if !seen {
		t.Fatal("other website lists dropped the recloud website")
	}

	blocked := callJSON(t, mux, http.MethodPost, "/api/accounts", `{"website_id":`+strconv.Itoa(websiteID)+`,"name":"nope","cookie":"x","status":"active"}`)
	if blocked.Code != 400 {
		t.Fatalf("mapped account save status %d", blocked.Code)
	}

	const nextCookie = "next-cookie-value"
	saved := callJSON(t, mux, http.MethodPost, "/api/recloud", `{"website_id":`+strconv.Itoa(websiteID)+`,"upstream_url":"https://next.example/app","cookie":"`+nextCookie+`"}`)
	if saved.Code != 200 {
		t.Fatalf("save status %d", saved.Code)
	}
	var gotURL, gotCookie string
	if err := db.QueryRow(`SELECT upstream_url FROM websites WHERE id=?`, websiteID).Scan(&gotURL); err != nil {
		t.Fatal(err)
	}
	if gotURL != "https://next.example/app" {
		t.Fatal("upstream URL was not stored for the next request")
	}
	if err := db.QueryRow(`SELECT cookie FROM accounts WHERE website_id=?`, websiteID).Scan(&gotCookie); err != nil {
		t.Fatal(err)
	}
	if gotCookie != nextCookie {
		t.Fatal("cookie was not stored for the next request")
	}
	var proxy, agent string
	if err := db.QueryRow(`SELECT proxy, user_agent FROM accounts WHERE website_id=?`, websiteID).Scan(&proxy, &agent); err != nil {
		t.Fatal(err)
	}
	if proxy != "" || agent != "" {
		t.Fatal("recloud save wrote a proxy or user agent")
	}

	rejected := callJSON(t, mux, http.MethodPost, "/api/recloud", `{"website_id":`+strconv.Itoa(websiteID)+`,"upstream_url":"https://user:secret@next.example","cookie":"x"}`)
	if rejected.Code != 400 {
		t.Fatalf("credential URL status %d", rejected.Code)
	}
}

func openRecloudTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "panel.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	body, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(body)); err != nil {
		t.Fatal(err)
	}
	ensureRecloudColumns(db)
	now := time.Now().UTC().Format(time.RFC3339)
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO operators (username, password_hash, role, status) VALUES ('master', 'x', 'master', 'active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO admin_sessions (token, operator_id, expires_at) VALUES ('recloud-test', 1, ?)`, expires); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`INSERT INTO tools (name, category, limits_json) VALUES ('Sem One', 'recloud', '[]')`)
	if err != nil {
		t.Fatal(err)
	}
	toolID, _ := res.LastInsertId()
	if _, err := db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url)
		VALUES (?, 'Sem One', 'tzsem1.gt4rents.com', 'secret', 30, '{}', 1, 'recloud', 'https://old.example')`, toolID); err != nil {
		t.Fatal(err)
	}
	other, err := db.Exec(`INSERT INTO tools (name, category, limits_json) VALUES ('Canva', 'design', '[]')`)
	if err != nil {
		t.Fatal(err)
	}
	otherTool, _ := other.LastInsertId()
	if _, err := db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled)
		VALUES (?, 'Canva', 'cnva.gt4rents.com', 'secret', 30, '{}', 1)`, otherTool); err != nil {
		t.Fatal(err)
	}
	var otherWebsite int
	if err := db.QueryRow(`SELECT id FROM websites WHERE kind=''`).Scan(&otherWebsite); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO accounts (website_id, name, cookie, user_agent, proxy, status, show_limit, description, automation_task_uid, automation_ingest_key, cookie_updated_at, last_used_at)
		VALUES (?, 'kept', 'kept-cookie', '', '', 'active', 1, '', '', '', ?, '')`, otherWebsite, now); err != nil {
		t.Fatal(err)
	}
	return db
}

func callJSON(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer recloud-test")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestRecloudListSearchAndCookieFilter(t *testing.T) {
	t.Setenv("GT4RENTS_MYSQL_PASSWORD", "")
	db := openRecloudTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO tools (name, category, limits_json) VALUES ('Sem Two', 'recloud', '[]')`)
	if err != nil {
		t.Fatal(err)
	}
	toolID, _ := res.LastInsertId()
	if _, err := db.Exec(`INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url)
		VALUES (?, 'Sem Two', 'tzsem2.gt4rents.com', 'secret', 30, '{}', 1, 'recloud', 'https://two.example')`, toolID); err != nil {
		t.Fatal(err)
	}
	var websiteID int
	if err := db.QueryRow(`SELECT id FROM websites WHERE domain='tzsem2.gt4rents.com'`).Scan(&websiteID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO accounts (website_id, name, cookie, user_agent, proxy, status, show_limit, description, automation_task_uid, automation_ingest_key, cookie_updated_at, last_used_at)
		VALUES (?, 'seat', 'secret-cookie-value', '', '', 'active', 1, '', '', '', ?, '')`, websiteID, now); err != nil {
		t.Fatal(err)
	}

	s := &server{db: db}
	mux := http.NewServeMux()
	s.routes(mux)

	all := recloudList(t, mux, "/api/recloud?page=1&pageSize=50&query=Sem&cookie=all")
	if all.Total != 2 || len(all.Items) != 2 {
		t.Fatalf("search total %d items %d", all.Total, len(all.Items))
	}
	saved := recloudList(t, mux, "/api/recloud?page=1&pageSize=50&cookie=saved")
	if saved.Total != 1 || saved.Items[0].ToolName != "Sem Two" {
		t.Fatalf("saved filter %#v", saved.Items)
	}
	missing := recloudList(t, mux, "/api/recloud?page=1&pageSize=50&cookie=missing")
	if missing.Total != 1 || missing.Items[0].ToolName != "Sem One" {
		t.Fatalf("missing filter %#v", missing.Items)
	}
	byURL := recloudList(t, mux, "/api/recloud?page=1&pageSize=50&query=two.example")
	if byURL.Total != 1 || byURL.Items[0].ToolName != "Sem Two" {
		t.Fatalf("url search %#v", byURL.Items)
	}
	page := recloudList(t, mux, "/api/recloud?page=1&pageSize=1")
	if page.Total != 2 || len(page.Items) != 1 || page.PageSize != 1 {
		t.Fatalf("page total %d len %d size %d", page.Total, len(page.Items), page.PageSize)
	}
}

func recloudList(t *testing.T, handler http.Handler, path string) struct {
	Total    int `json:"total"`
	PageSize int `json:"pageSize"`
	Items    []struct {
		ToolName string `json:"tool_name"`
	} `json:"items"`
} {
	t.Helper()
	rec := callJSON(t, handler, http.MethodGet, path, "")
	if rec.Code != 200 {
		t.Fatalf("%s status %d", path, rec.Code)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("secret-cookie-value")) || bytes.Contains(rec.Body.Bytes(), []byte("kept-cookie")) {
		t.Fatal("recloud list included a cookie")
	}
	var page struct {
		Total    int `json:"total"`
		PageSize int `json:"pageSize"`
		Items    []struct {
			ToolName string `json:"tool_name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	return page
}
