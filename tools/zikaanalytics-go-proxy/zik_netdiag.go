package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxRecentZikLogs = 400

// ProxyLogEntry is a ring-buffer row for /__logs (browser + upstream diagnostics).
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
}

var (
	recentZikLogs   []ProxyLogEntry
	recentZikLogsMu sync.Mutex
)

func pushProxyLog(e ProxyLogEntry) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	if e.Level == "" {
		e.Level = "info"
	}
	recentZikLogsMu.Lock()
	recentZikLogs = append(recentZikLogs, e)
	if len(recentZikLogs) > maxRecentZikLogs {
		recentZikLogs = recentZikLogs[len(recentZikLogs)-maxRecentZikLogs:]
	}
	recentZikLogsMu.Unlock()

	log.Printf("[%s] %s %s → %d user=%s account=%s %s",
		e.Source, e.Method, e.Path, e.Status, e.User, e.Account, e.Detail)
}

func snapshotLogs() []ProxyLogEntry {
	recentZikLogsMu.Lock()
	defer recentZikLogsMu.Unlock()
	out := make([]ProxyLogEntry, len(recentZikLogs))
	copy(out, recentZikLogs)
	return out
}

func truncateForLog(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func snippetForLog(body []byte, contentType string) string {
	ct := strings.ToLower(contentType)
	if !(strings.Contains(ct, "json") || strings.Contains(ct, "text") || ct == "" || strings.Contains(ct, "javascript")) {
		return fmt.Sprintf("(%d bytes non-text)", len(body))
	}
	return truncateForLog(string(body), 280)
}

func zikShouldDiagUpstream(status int, path string) bool {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return true
	}
	if status >= 400 {
		return true
	}
	if status >= 300 && status < 400 {
		p := strings.ToLower(path)
		return p == "/" || p == "" || strings.Contains(p, "login") || strings.Contains(p, "signin") ||
			strings.Contains(p, "dashboard")
	}
	return false
}

func zikClientDiagHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 12<<10))
	user := ""
	account := ""
	if u, err := panelSessionUsername(r); err == nil {
		user = u
		if tok, _, ok := sessionFromRequest(r); ok {
			if acc, aerr := loadPanelSessionAccount(loadConfig(), tok); aerr == nil {
				account = acc.Name
			}
		}
	}
	detail := truncateForLog(string(body), 800)
	log.Printf("[CLIENT-DIAG] user=%s account=%s %s", user, account, detail)
	pushProxyLog(ProxyLogEntry{
		Source:  "CLIENT",
		Level:   "warn",
		Method:  "POST",
		Path:    "/api/client-diag",
		Status:  200,
		User:    user,
		Account: account,
		Detail:  detail,
	})
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"ok"}`)
}

func liveLogsPageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, `<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Zik Live Logs</title>
<style>
*{box-sizing:border-box}body{margin:0;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;background:#0b1220;color:#e5eefc}
.wrap{max-width:1200px;margin:0 auto;padding:20px}
h1{font-size:18px;margin:0 0 8px}p{color:#93a4bd;font-size:13px;margin:0 0 16px;line-height:1.45}
.bar{display:flex;gap:10px;align-items:center;margin-bottom:14px;flex-wrap:wrap}
button,a.btn{background:#1d4ed8;color:#fff;border:0;border-radius:8px;padding:8px 12px;font:inherit;cursor:pointer;text-decoration:none}
button.secondary{background:#1f2937}
.meta{color:#93a4bd;font-size:12px}
table{width:100%;border-collapse:collapse;font-size:12px}
th,td{border-bottom:1px solid #1e293b;padding:8px 6px;vertical-align:top;text-align:left}
th{color:#93a4bd;font-weight:600}
.s401,.s403{color:#fb7185}.s4xx{color:#fbbf24}.sok{color:#4ade80}
.detail{color:#cbd5e1;word-break:break-word;max-width:520px}
</style></head><body><div class="wrap">
<h1>Zik Analytics — live logs</h1>
<p>CLIENT-DIAG (browser nav / 401 / soft-401) + NET (upstream). Copy rows or use JSON. Server: <code>tail -f app.log | grep -E 'CLIENT-DIAG|NET|NAV|FAILOVER|ZIK|PROXY'</code></p>
<div class="bar">
  <button onclick="loadLogs()">Refresh</button>
  <a class="btn secondary" href="/__logs.json" target="_blank">JSON</a>
  <span class="meta" id="meta">loading…</span>
</div>
<table><thead><tr>
  <th>Time</th><th>Src</th><th>Status</th><th>Method</th><th>Path</th><th>User</th><th>Detail</th>
</tr></thead><tbody id="rows"></tbody></table>
<script>
async function loadLogs(){
  try{
    const r=await fetch('/__logs.json?t='+Date.now());
    const data=await r.json();
    const rows=(data.logs||[]).slice().reverse();
    document.getElementById('meta').textContent=rows.length+' entries · updated '+new Date().toLocaleTimeString();
    document.getElementById('rows').innerHTML=rows.map(e=>{
      const cls=e.status===401||e.status===403?'s401':(e.status>=400?'s4xx':(e.status? 'sok':''));
      return '<tr><td>'+new Date(e.time).toLocaleTimeString()+'</td><td>'+(e.source||'')+'</td><td class="'+cls+'">'+(e.status||'')+'</td><td>'+(e.method||'')+'</td><td>'+(e.path||'')+'</td><td>'+(e.user||'')+' / '+(e.account||'')+'</td><td class="detail">'+(e.detail||'')+'</td></tr>';
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

// zikSoftNoisy401Script runs FIRST so SPA auth interceptors never see BestSellers 401
// (that 401 is normal on some plans and was causing dashboard reload storms).
func zikSoftNoisy401Script() string {
	return `<script data-tm-zik-soft401="1">
(function(){
  if (window.__zikSoft401) return;
  window.__zikSoft401 = true;
  function isNoisy(u){
    u = String(u || '').toLowerCase();
    return u.indexOf('getebayweeklybestsellers') !== -1 || u.indexOf('weeklybestsellers') !== -1 ||
      (u.indexOf('bestsellers') !== -1 && u.indexOf('dashboard') !== -1);
  }
  function softOK(){
    return new Response('null', {status:200, statusText:'OK', headers:{'Content-Type':'application/json'}});
  }
  function report(kind, info){
    try {
      var payload = {kind: kind, t: Date.now(), href: location.href};
      if (info) for (var k in info) if (Object.prototype.hasOwnProperty.call(info, k)) payload[k] = info[k];
      var body = JSON.stringify(payload);
      if (navigator.sendBeacon) {
        navigator.sendBeacon('/api/client-diag', new Blob([body], {type:'application/json'}));
        return;
      }
      fetch('/api/client-diag', {method:'POST', credentials:'same-origin',
        headers:{'Content-Type':'application/json'}, body: body, keepalive:true}).catch(function(){});
    } catch (e) {}
  }
  var fo = window.fetch;
  if (typeof fo === 'function') {
    window.fetch = function(input, init){
      var u = '';
      try {
        if (typeof input === 'string') u = input;
        else if (input && input.url) u = String(input.url);
      } catch (e) {}
      return fo.apply(this, arguments).then(function(res){
        try {
          if (res && (res.status === 401 || res.status === 403) && isNoisy(u)) {
            report('soft401', {url: u, status: res.status});
            return softOK();
          }
          if (res && (res.status === 401 || res.status === 403)) {
            report('api401', {url: u, status: res.status});
          }
        } catch (e2) {}
        return res;
      });
    };
  }
  var XO = XMLHttpRequest.prototype.open;
  var XS = XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.open = function(m, u){
    try { this.__zikURL = String(u || ''); } catch (e) { this.__zikURL = ''; }
    return XO.apply(this, arguments);
  };
  XMLHttpRequest.prototype.send = function(){
    var xhr = this;
    var url = xhr.__zikURL || '';
    if (isNoisy(url)) {
      xhr.addEventListener('load', function(){
        try {
          if (xhr.status === 401 || xhr.status === 403) {
            report('soft401_xhr', {url: url, status: xhr.status});
            try {
              Object.defineProperty(xhr, 'status', {configurable:true, get:function(){ return 200; }});
              Object.defineProperty(xhr, 'statusText', {configurable:true, get:function(){ return 'OK'; }});
              Object.defineProperty(xhr, 'responseText', {configurable:true, get:function(){ return 'null'; }});
              Object.defineProperty(xhr, 'response', {configurable:true, get:function(){ return 'null'; }});
            } catch (e) {}
          }
        } catch (e2) {}
      });
    } else {
      xhr.addEventListener('load', function(){
        try {
          if (xhr.status === 401 || xhr.status === 403) {
            report('api401_xhr', {url: url, status: xhr.status});
          }
        } catch (e) {}
      });
    }
    return XS.apply(this, arguments);
  };
})();
</script>`
}

// zikClientNetDiagScript reports hard navigations / login bounces to app.log.
func zikClientNetDiagScript() string {
	return `<script data-tm-zik-netdiag="1">
(function(){
  if (window.__zikNetDiag) return;
  window.__zikNetDiag = true;
  var sent = 0, MAX = 120;
  function report(kind, info){
    if (sent >= MAX) return;
    sent++;
    try {
      var payload = {kind: kind, t: Date.now(), href: location.href, path: location.pathname};
      if (info) for (var k in info) if (Object.prototype.hasOwnProperty.call(info, k)) payload[k] = info[k];
      var body = JSON.stringify(payload);
      if (navigator.sendBeacon) {
        navigator.sendBeacon('/api/client-diag', new Blob([body], {type:'application/json'}));
        return;
      }
      fetch('/api/client-diag', {method:'POST', credentials:'same-origin',
        headers:{'Content-Type':'application/json'}, body: body, keepalive:true}).catch(function(){});
    } catch (e) {}
  }
  report('boot', {referrer: document.referrer || ''});
  var last = location.href;
  setInterval(function(){
    if (location.href !== last) {
      var prev = last;
      last = location.href;
      report('nav_change', {from: prev, to: last});
    }
  }, 400);
  var _ps = history.pushState.bind(history);
  history.pushState = function(){
    try { report('pushState', {url: arguments[2]}); } catch (e) {}
    return _ps.apply(history, arguments);
  };
  var _rs = history.replaceState.bind(history);
  history.replaceState = function(){
    try { report('replaceState', {url: arguments[2]}); } catch (e) {}
    return _rs.apply(history, arguments);
  };
  window.addEventListener('popstate', function(){ report('popstate', {}); });
  window.addEventListener('beforeunload', function(){ report('beforeunload', {}); });
})();
</script>`
}
