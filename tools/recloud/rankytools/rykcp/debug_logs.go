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

const maxRecentLogs = 400

type proxyLogEntry struct {
	Time   time.Time `json:"time"`
	Source string    `json:"source"`
	Method string    `json:"method"`
	Path   string    `json:"path"`
	Status int       `json:"status"`
	Ms     int64     `json:"ms"`
	Detail string    `json:"detail,omitempty"`
}

var (
	recentLogs   []proxyLogEntry
	recentLogsMu sync.Mutex
)

func pushProxyLog(e proxyLogEntry) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	recentLogsMu.Lock()
	recentLogs = append(recentLogs, e)
	if len(recentLogs) > maxRecentLogs {
		recentLogs = recentLogs[len(recentLogs)-maxRecentLogs:]
	}
	recentLogsMu.Unlock()
	log.Printf("[%s] %s %s → %d %dms %s", e.Source, e.Method, e.Path, e.Status, e.Ms, e.Detail)
}

func snapshotLogs() []proxyLogEntry {
	recentLogsMu.Lock()
	defer recentLogsMu.Unlock()
	out := make([]proxyLogEntry, len(recentLogs))
	copy(out, recentLogs)
	return out
}

func liveLogsPageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, `<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>rykcp live logs</title>
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
.s401,.s403,.s5xx{color:#fb7185}.s4xx{color:#fbbf24}.sok{color:#4ade80}
.detail{color:#cbd5e1;word-break:break-word;max-width:480px}
</style></head><body><div class="wrap">
<h1>RankyTools ChatGPT (rykcp) live logs</h1>
<p>Recent proxy hops (HTML/API/WS). Auto-refresh 3s. Copy this page or open JSON when reporting a hang.</p>
<div class="bar">
  <button onclick="loadLogs()">Refresh</button>
  <a class="btn secondary" href="/__logs.json" target="_blank">JSON</a>
  <span class="meta" id="meta">loading…</span>
</div>
<table><thead><tr>
  <th>Time</th><th>Status</th><th>ms</th><th>Method</th><th>Path</th><th>Source</th><th>Detail</th>
</tr></thead><tbody id="rows"></tbody></table>
<script>
async function loadLogs(){
  try{
    const r=await fetch('/__logs.json?t='+Date.now());
    const data=await r.json();
    const rows=(data.logs||[]).slice().reverse();
    document.getElementById('meta').textContent=rows.length+' entries · updated '+new Date().toLocaleTimeString();
    document.getElementById('rows').innerHTML=rows.map(e=>{
      const cls=e.status>=500?'s5xx':(e.status===401||e.status===403?'s401':(e.status>=400?'s4xx':'sok'));
      return '<tr><td>'+new Date(e.time).toLocaleTimeString()+'</td><td class="'+cls+'">'+e.status+'</td><td>'+(e.ms||0)+'</td><td>'+e.method+'</td><td>'+e.path+'</td><td>'+e.source+'</td><td class="detail">'+(e.detail||'')+'</td></tr>';
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

func shrinkCookiesHandler(w http.ResponseWriter, r *http.Request) {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if allowShrinkOrigin(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Clear-Site-Data", `"cookies", "clientHints"`)
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	pushProxyLog(proxyLogEntry{
		Source: "shrink",
		Method: r.Method,
		Path:   "/__tm_shrink",
		Status: 204,
		Detail: "cleared browser jar for host",
	})
	w.WriteHeader(http.StatusNoContent)
}

func allowShrinkOrigin(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return false
	}
	lower := strings.ToLower(origin)
	return strings.HasSuffix(lower, ".upays.in") ||
		lower == "https://upays.in" ||
		strings.HasSuffix(lower, ".gt4rents.com") ||
		lower == "https://gt4rents.com" ||
		strings.HasPrefix(lower, "http://127.0.0.1") ||
		strings.HasPrefix(lower, "http://localhost")
}
