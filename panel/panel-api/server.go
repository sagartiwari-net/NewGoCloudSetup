package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type operator struct {
	ID       int
	Username string
	Role     string
	Status   string
}

func main() {
	db := openDB("data/panel.db")
	s := &server{db: db}
	mux := http.NewServeMux()
	s.routes(mux)
	log.Printf("panel api on http://127.0.0.1:8090")
	log.Fatal(http.ListenAndServe("127.0.0.1:8090", s.withCORS(mux)))
}

type server struct{ db *sql.DB }

func (s *server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.HandleFunc("GET /api/me", s.me)
	mux.HandleFunc("POST /api/password", s.changePassword)
	mux.HandleFunc("GET /api/dashboard", s.dashboard)
	mux.HandleFunc("GET /api/tools", s.listTools)
	mux.HandleFunc("POST /api/tools", s.saveTool)
	mux.HandleFunc("GET /api/websites", s.listWebsites)
	mux.HandleFunc("POST /api/websites", s.saveWebsite)
	mux.HandleFunc("DELETE /api/websites", s.deleteWebsite)
	mux.HandleFunc("GET /api/accounts", s.listAccounts)
	mux.HandleFunc("GET /api/accounts/{id}", s.getAccount)
	mux.HandleFunc("POST /api/accounts", s.saveAccount)
	mux.HandleFunc("DELETE /api/accounts", s.deleteAccount)
	mux.HandleFunc("POST /api/accounts/proxy", s.assignProxy)
	mux.HandleFunc("POST /api/accounts/user-agent", s.assignUserAgent)
	mux.HandleFunc("GET /api/users", s.listUsers)
	mux.HandleFunc("GET /api/directory-users", s.listDirectoryUsers)
	mux.HandleFunc("POST /api/users", s.saveUser)
	mux.HandleFunc("DELETE /api/users", s.deleteUser)
	mux.HandleFunc("POST /api/users/{id}/reset", s.resetUser)
	mux.HandleFunc("GET /api/sessions", s.listSessions)
	mux.HandleFunc("POST /api/sessions", s.openSession)
	mux.HandleFunc("POST /api/sessions/assign", s.assignSession)
	mux.HandleFunc("DELETE /api/sessions", s.endSession)
	mux.HandleFunc("GET /api/quota", s.listQuota)
	mux.HandleFunc("GET /api/logins", s.listLogins)
	mux.HandleFunc("DELETE /api/logins", s.clearLogins)
	mux.HandleFunc("GET /api/switches", s.listSwitches)
	mux.HandleFunc("DELETE /api/switches", s.clearSwitches)
	mux.HandleFunc("GET /api/extension-events", s.listExtensionEvents)
	mux.HandleFunc("DELETE /api/extension-events", s.clearExtensionEvents)
	mux.HandleFunc("GET /api/analytics/summary", s.analyticsSummary)
	mux.HandleFunc("GET /api/user-report", s.userReport)
	mux.HandleFunc("GET /api/user-report/events", s.userReportEvents)
	mux.HandleFunc("GET /api/ip-report", s.ipReport)
	mux.HandleFunc("GET /api/automate-tasks", s.listAutomate)
	mux.HandleFunc("GET /api/ingest-logs", s.emptyPage)
	mux.HandleFunc("GET /api/products", s.listProducts)
	mux.HandleFunc("POST /api/products", s.saveProduct)
	mux.HandleFunc("DELETE /api/products", s.deleteID("products"))
	mux.HandleFunc("GET /api/violations", s.listViolations)
	mux.HandleFunc("GET /api/security", s.listSecurity)
	mux.HandleFunc("GET /api/blocked-ips", s.listBlocked)
	mux.HandleFunc("POST /api/blocked-ips", s.blockIP)
	mux.HandleFunc("DELETE /api/blocked-ips", s.deleteID("blocked_ips"))
	mux.HandleFunc("GET /api/proxies", s.listProxies)
	mux.HandleFunc("POST /api/proxies", s.saveProxy)
	mux.HandleFunc("DELETE /api/proxies", s.deleteID("proxies"))
	mux.HandleFunc("GET /api/user-agents", s.listAgents)
	mux.HandleFunc("POST /api/user-agents", s.saveAgent)
	mux.HandleFunc("DELETE /api/user-agents", s.deleteID("user_agents"))
	mux.HandleFunc("GET /api/resellers", s.listResellers)
	mux.HandleFunc("POST /api/resellers", s.saveReseller)
	mux.HandleFunc("DELETE /api/resellers", s.deleteReseller)
	mux.HandleFunc("GET /api/telegram/settings", s.telegramSettings)
	mux.HandleFunc("POST /api/telegram/token", s.saveSetting("bot_token"))
	mux.HandleFunc("POST /api/telegram/repeat", s.saveRepeat)
	mux.HandleFunc("GET /api/telegram/templates", s.telegramTemplates)
	mux.HandleFunc("POST /api/telegram/templates", s.saveTemplates)
	mux.HandleFunc("GET /api/telegram/destinations", s.listDestinations)
	mux.HandleFunc("POST /api/telegram/destinations", s.saveDestination)
	mux.HandleFunc("DELETE /api/telegram/destinations", s.deleteDestination)
	mux.HandleFunc("GET /api/telegram/routes", s.listRoutes)
	mux.HandleFunc("POST /api/telegram/routes", s.saveRoute)
	mux.HandleFunc("GET /api/telegram/deliveries", s.listDeliveries)
	mux.HandleFunc("DELETE /api/telegram/deliveries", s.clearDeliveries)
	mux.HandleFunc("GET /api/spam-reports", s.listSpamReports)
	mux.HandleFunc("POST /api/spam-rule", s.saveSpamRule)
	mux.HandleFunc("GET /api/websites/{id}/next-account", s.nextAccount)
	mux.HandleFunc("POST /api/websites/{id}/use-account", s.useAccount)
	mux.HandleFunc("POST /api/sessions/check", s.checkSession)
	mux.HandleFunc("POST /api/host-reports", s.hostReport)
	mux.HandleFunc("GET /api/host-reports", s.listHostReports)
	mux.HandleFunc("GET /api/access-check", s.accessCheck)
	mux.HandleFunc("POST /api/access", s.openAccess)
	mux.HandleFunc("GET /api/access-alerts", s.accessAlerts)
	mux.HandleFunc("POST /api/access-alerts", s.saveAccessAlerts)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *server) ok(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.auth(w, r); !ok {
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) emptyList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.auth(w, r); !ok {
		return
	}
	writeJSON(w, 200, []any{})
}

func (s *server) emptyPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.auth(w, r); !ok {
		return
	}
	page, size, _ := pageQuery(r)
	writeJSON(w, 200, map[string]any{"items": []any{}, "total": 0, "page": page, "pageSize": size})
}

func pageQuery(r *http.Request) (int, int, string) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if size < 1 || size > 200 {
		size = 8
	}
	return page, size, strings.TrimSpace(r.URL.Query().Get("query"))
}

func (s *server) auth(w http.ResponseWriter, r *http.Request) (operator, bool) {
	token := ""
	if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
		token = strings.TrimPrefix(header, "Bearer ")
	}
	if token == "" {
		if c, err := r.Cookie("panel_session"); err == nil {
			token = c.Value
		}
	}
	if token == "" {
		writeErr(w, http.StatusUnauthorized, "Sign in required")
		return operator{}, false
	}
	var op operator
	var expires string
	err := s.db.QueryRow(`SELECT o.id, o.username, o.role, o.status, s.expires_at
		FROM admin_sessions s JOIN operators o ON o.id = s.operator_id WHERE s.token = ?`, token).
		Scan(&op.ID, &op.Username, &op.Role, &op.Status, &expires)
	if err != nil || op.Status != "active" {
		writeErr(w, http.StatusUnauthorized, "Sign in required")
		return operator{}, false
	}
	return op, true
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "Invalid body")
		return
	}
	var op operator
	var hash string
	err := s.db.QueryRow(`SELECT id, username, role, status, password_hash FROM operators WHERE username = ?`, strings.TrimSpace(body.Username)).
		Scan(&op.ID, &op.Username, &op.Role, &op.Status, &hash)
	if err != nil || hash != hashPassword(body.Password) || op.Status != "active" {
		writeErr(w, 401, "Invalid username or password")
		return
	}
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	token := hex.EncodeToString(buf)
	expires := time.Now().Add(7 * 24 * time.Hour).UTC().Format(time.RFC3339)
	_, _ = s.db.Exec(`INSERT INTO admin_sessions (token, operator_id, expires_at) VALUES (?,?,?)`, token, op.ID, expires)
	http.SetCookie(w, &http.Cookie{Name: "panel_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 7 * 24 * 3600})
	bodyOut := s.operatorJSON(op)
	bodyOut["token"] = token
	writeJSON(w, 200, bodyOut)
}

func (s *server) operatorJSON(op operator) map[string]any {
	ids := []int{}
	if op.Role == "master" {
		rows, err := s.db.Query(`SELECT id FROM websites ORDER BY id`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id int
				_ = rows.Scan(&id)
				ids = append(ids, id)
			}
		}
	} else {
		rows, err := s.db.Query(`SELECT website_id FROM operator_websites WHERE operator_id = ? ORDER BY website_id`, op.ID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id int
				_ = rows.Scan(&id)
				ids = append(ids, id)
			}
		}
	}
	return map[string]any{"id": op.ID, "username": op.Username, "role": op.Role, "website_ids": ids}
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("panel_session"); err == nil {
		_, _ = s.db.Exec(`DELETE FROM admin_sessions WHERE token = ?`, c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "panel_session", Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, s.operatorJSON(op))
}

func (s *server) changePassword(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok || op.Role != "master" {
		if ok {
			writeErr(w, 403, "Master only")
		}
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if len(body.Password) < 8 {
		writeErr(w, 400, "Use at least 8 characters")
		return
	}
	_, _ = s.db.Exec(`UPDATE operators SET password_hash = ? WHERE id = ?`, hashPassword(body.Password), op.ID)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) dashboard(w http.ResponseWriter, r *http.Request) {
	op, ok := s.auth(w, r)
	if !ok {
		return
	}
	s.purgeOldRows()
	now := time.Now().UTC().Format(time.RFC3339)
	accScope, accArgs := s.scope(op, "website_id")
	sessScope, sessArgs := s.scope(op, "website_id")
	useScope, useArgs := s.scope(op, "website_id")
	liveScope, liveArgs := s.scope(op, "s.website_id")
	loginScope, loginArgs := s.scope(op, "website_id")
	var accounts, active, hits int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE `+accScope, accArgs...).Scan(&accounts)
	activeArgs := append([]any{now}, sessArgs...)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM live_sessions WHERE expires_at > ? AND `+sessScope, activeArgs...).Scan(&active)
	today := time.Now().UTC().Format("2006-01-02")
	hitArgs := append([]any{today}, useArgs...)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM usage_events WHERE timestamp >= ? AND `+useScope, hitArgs...).Scan(&hits)
	liveArgs = append([]any{now}, liveArgs...)
	rows, err := s.db.Query(`SELECT s.username, s.website_id, w.name, s.client_ip, s.created_at, s.expires_at, COALESCE(a.name,'Auto')
		FROM live_sessions s JOIN websites w ON w.id=s.website_id LEFT JOIN accounts a ON a.id=s.assigned_account_id
		WHERE s.expires_at > ? AND `+liveScope+` ORDER BY s.id DESC LIMIT 8`, liveArgs...)
	users := []any{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var websiteID int
			var username, website, ip, created, expires, account string
			if rows.Scan(&username, &websiteID, &website, &ip, &created, &expires, &account) == nil {
				users = append(users, map[string]any{
					"username": username, "website_id": websiteID, "website_name": website, "client_ip": ip,
					"created_at": created, "expires_at": expires, "assigned_account_name": account,
				})
			}
		}
	}
	chart := []any{}
	for i := 6; i >= 0; i-- {
		day := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		var n int
		dayArgs := append([]any{day}, loginArgs...)
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM login_events WHERE substr(logged_in_at,1,10)=? AND `+loginScope, dayArgs...).Scan(&n)
		chart = append(chart, map[string]any{"day": day, "hits": n})
	}
	writeJSON(w, 200, map[string]any{
		"active_sessions": active, "accounts_total": accounts, "credit_hits_today": hits,
		"active_users_list": users, "credit_hits_7d": chart,
		"attention": map[string]any{
			"cookies":  map[string]any{"count": 0, "items": []any{}},
			"limits":   map[string]any{"count": 0, "items": []any{}},
			"expiring": map[string]any{"count": 0, "items": []any{}},
		},
	})
}

func readBody(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}
