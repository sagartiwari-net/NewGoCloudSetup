package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxRecentLogs = 300

type ProxyLogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Source  string    `json:"source"`
	Method  string    `json:"method"`
	Path    string    `json:"path"`
	Status  int       `json:"status"`
	User    string    `json:"user,omitempty"`
	Account string    `json:"account,omitempty"`
	Detail  string    `json:"detail,omitempty"`
	HasUID  bool      `json:"has_x_uid"`
	HasAPI  bool      `json:"has_x_api_token"`
	CookieN int       `json:"cookie_names"`
}

var (
	recentLogs   []ProxyLogEntry
	recentLogsMu sync.Mutex
)

func pushProxyLog(e ProxyLogEntry) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	recentLogsMu.Lock()
	recentLogs = append(recentLogs, e)
	if len(recentLogs) > maxRecentLogs {
		recentLogs = recentLogs[len(recentLogs)-maxRecentLogs:]
	}
	recentLogsMu.Unlock()

	log.Printf("[%s] %s %s → %d user=%s account=%s uid=%v apiTok=%v cookies=%d %s",
		e.Source, e.Method, e.Path, e.Status, e.User, e.Account, e.HasUID, e.HasAPI, e.CookieN, e.Detail)
}

func snapshotLogs() []ProxyLogEntry {
	recentLogsMu.Lock()
	defer recentLogsMu.Unlock()
	out := make([]ProxyLogEntry, len(recentLogs))
	copy(out, recentLogs)
	return out
}

func countCookieNames(cookieHeader string) int {
	if strings.TrimSpace(cookieHeader) == "" {
		return 0
	}
	n := 0
	for _, part := range strings.Split(cookieHeader, ";") {
		if strings.Contains(part, "=") {
			n++
		}
	}
	return n
}

func shouldLogUpstreamStatus(status int, path string) bool {
	if status == 401 || status == 403 {
		return true
	}
	if status >= 400 && (strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/r/api/")) {
		return true
	}
	return false
}

func snippetForLog(body []byte, contentType string) string {
	ct := strings.ToLower(contentType)
	if !(strings.Contains(ct, "json") || strings.Contains(ct, "text") || ct == "") {
		return fmt.Sprintf("(%d bytes non-text)", len(body))
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 240 {
		s = s[:240] + "…"
	}
	return s
}

func liveLogsPageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, `<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>SellerAmp Live Logs</title>
<style>
*{box-sizing:border-box}body{margin:0;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;background:#0b1220;color:#e5eefc}
.wrap{max-width:1100px;margin:0 auto;padding:20px}
h1{font-size:18px;margin:0 0 8px}p{color:#93a4bd;font-size:13px;margin:0 0 16px}
.bar{display:flex;gap:10px;align-items:center;margin-bottom:14px;flex-wrap:wrap}
button,a.btn{background:#1d4ed8;color:#fff;border:0;border-radius:8px;padding:8px 12px;font:inherit;cursor:pointer;text-decoration:none}
button.secondary{background:#1f2937}
.meta{color:#93a4bd;font-size:12px}
table{width:100%;border-collapse:collapse;font-size:12px}
th,td{border-bottom:1px solid #1e293b;padding:8px 6px;vertical-align:top;text-align:left}
th{color:#93a4bd;font-weight:600}
.s401,.s403{color:#fb7185}.s4xx{color:#fbbf24}.sok{color:#4ade80}
.detail{color:#cbd5e1;word-break:break-word;max-width:420px}
</style></head><body><div class="wrap">
<h1>SellerAmp live error logs</h1>
<p>Shows recent upstream 401/403 and /api/* failures. Auto-refreshes every 3s.</p>
<div class="bar">
  <button onclick="loadLogs()">Refresh</button>
  <a class="btn secondary" href="/__logs.json" target="_blank">JSON</a>
  <span class="meta" id="meta">loading…</span>
</div>
<table><thead><tr>
  <th>Time</th><th>Status</th><th>Method</th><th>Path</th><th>Auth</th><th>Detail</th>
</tr></thead><tbody id="rows"></tbody></table>
<script>
async function loadLogs(){
  try{
    const r=await fetch('/__logs.json?t='+Date.now());
    const data=await r.json();
    const rows=(data.logs||[]).slice().reverse();
    document.getElementById('meta').textContent=rows.length+' entries · updated '+new Date().toLocaleTimeString();
    document.getElementById('rows').innerHTML=rows.map(e=>{
      const cls=e.status===401||e.status===403?'s401':(e.status>=400?'s4xx':'sok');
      const auth='uid='+(e.has_x_uid?'Y':'N')+' token='+(e.has_x_api_token?'Y':'N')+' cookies='+(e.cookie_names||0);
      return '<tr><td>'+new Date(e.time).toLocaleTimeString()+'</td><td class="'+cls+'">'+e.status+'</td><td>'+e.method+'</td><td>'+e.path+'</td><td>'+auth+'</td><td class="detail">'+(e.detail||'')+'</td></tr>';
    }).join('');
  }catch(err){
    document.getElementById('meta').textContent='failed: '+err;
  }
}
loadLogs();
setInterval(loadLogs,3000);
</script></div></body></html>`)
}

func liveLogsJSONHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"logs": snapshotLogs(),
	})
}
