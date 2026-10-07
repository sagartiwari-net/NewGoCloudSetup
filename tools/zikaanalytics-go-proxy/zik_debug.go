package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// zikDebugPageHandler — one page to see auth probe + recent NET/CLIENT errors + copy dump.
func zikDebugPageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, `<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Zik Debug</title>
<style>
*{box-sizing:border-box}body{margin:0;font-family:ui-sans-serif,system-ui,sans-serif;background:#0b1220;color:#e5eefc}
.wrap{max-width:1100px;margin:0 auto;padding:20px 16px 48px}
h1{font-size:22px;margin:0 0 6px} .sub{color:#93a4bd;font-size:13px;margin:0 0 18px;line-height:1.45}
.card{background:#111827;border:1px solid #1e293b;border-radius:14px;padding:16px 18px;margin-bottom:14px}
.card h2{font-size:14px;margin:0 0 10px;color:#93a4bd;font-weight:600;letter-spacing:.02em;text-transform:uppercase}
.row{display:flex;flex-wrap:wrap;gap:10px;margin-bottom:10px}
.pill{display:inline-flex;align-items:center;gap:6px;padding:6px 10px;border-radius:999px;font-size:12px;background:#1f2937;border:1px solid #334155}
.ok{color:#4ade80}.bad{color:#fb7185}.warn{color:#fbbf24}
button,.btn{background:#2563eb;color:#fff;border:0;border-radius:10px;padding:10px 14px;font:inherit;cursor:pointer;text-decoration:none;display:inline-block}
button.secondary{background:#1f2937}
button:disabled{opacity:.5;cursor:wait}
pre{white-space:pre-wrap;word-break:break-word;background:#0b1220;border:1px solid #1e293b;border-radius:10px;padding:12px;font-size:12px;line-height:1.45;max-height:420px;overflow:auto;margin:0}
table{width:100%;border-collapse:collapse;font-size:12px;font-family:ui-monospace,Menlo,monospace}
th,td{border-bottom:1px solid #1e293b;padding:8px 6px;vertical-align:top;text-align:left}
th{color:#93a4bd} .s401{color:#fb7185} .detail{color:#cbd5e1;max-width:480px;word-break:break-word}
.verdict{font-size:15px;line-height:1.5;padding:12px 14px;border-radius:12px;margin-top:8px}
.verdict.dead{background:#3f1d2e;border:1px solid #9f1239;color:#fecdd3}
.verdict.alive{background:#052e1a;border:1px solid #166534;color:#bbf7d0}
.verdict.unk{background:#1f2937;border:1px solid #334155;color:#e2e8f0}
</style></head><body><div class="wrap">
<h1>Zik Debug Desk</h1>
<p class="sub">Open this from the same browser tab that has your access session.
Use <b>Run probe</b>, then <b>Copy dump</b> and paste to support / Cursor.</p>

<div class="card">
  <h2>Session</h2>
  <div class="row" id="sessPills"><span class="pill">loading…</span></div>
  <div class="row">
    <button id="btnProbe" onclick="runProbe()">Run probe (GetStore + suggestions)</button>
    <button class="secondary" onclick="loadLogs()">Refresh logs</button>
    <button class="secondary" onclick="copyDump()">Copy dump</button>
    <a class="btn secondary" href="/__logs.json" target="_blank">Raw JSON</a>
  </div>
  <div id="verdict" class="verdict unk">Probe not run yet.</div>
</div>

<div class="card">
  <h2>Probe result</h2>
  <pre id="probeOut">Click “Run probe”.</pre>
</div>

<div class="card">
  <h2>Recent errors (NET / CLIENT / FAILOVER)</h2>
  <table><thead><tr><th>Time</th><th>Src</th><th>Status</th><th>Method</th><th>Path</th><th>Detail</th></tr></thead>
  <tbody id="rows"></tbody></table>
</div>

<script>
let lastProbe = null;
let lastLogs = [];
let lastSession = null;

function esc(s){ return String(s||'').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }

async function loadSession(){
  try{
    const r = await fetch('/__debug.json?t='+Date.now());
    const d = await r.json();
    lastSession = d.session || {};
    const s = lastSession;
    const pills = [];
    pills.push('<span class="pill">user: <b>'+esc(s.user||'—')+'</b></span>');
    pills.push('<span class="pill">account: <b>'+esc(s.account||'—')+'</b></span>');
    pills.push('<span class="pill">bearer: <b class="'+(s.bearer?'ok':'bad')+'">'+(s.bearer?'yes':'no')+'</b></span>');
    pills.push('<span class="pill">sessionCookie: <b class="'+(s.session_cookie?'ok':'bad')+'">'+(s.session_cookie?'yes':'no')+'</b></span>');
    pills.push('<span class="pill">jwtExpired: <b class="'+(s.jwt_expired?'bad':'ok')+'">'+(s.jwt_expired?'YES':'no')+'</b></span>');
    pills.push('<span class="pill">proxy: <b>'+(s.has_proxy?'yes':'no')+'</b></span>');
    pills.push('<span class="pill">claims: '+esc(s.claims||'')+'</span>');
    document.getElementById('sessPills').innerHTML = pills.join('');
    if (d.probe) {
      lastProbe = d.probe;
      showProbe(d.probe);
    }
  }catch(e){
    document.getElementById('sessPills').innerHTML = '<span class="pill bad">session load failed: '+esc(e)+' — open via access link first</span>';
  }
}

function showProbe(p){
  document.getElementById('probeOut').textContent = JSON.stringify(p, null, 2);
  const v = document.getElementById('verdict');
  if (p.research_ok) {
    v.className = 'verdict alive';
    v.textContent = 'OK — research API works with this account cookie. If UI still errors, hard-refresh or send the dump.';
  } else if (p.basic_ok && !p.research_ok) {
    v.className = 'verdict dead';
    v.textContent = 'SOFT-DEAD COOKIE — dashboard widgets may load, but research APIs return Unauthorized / Please log in again. Admin must re-login on real Zik, export GoAuto (cookies+localStorage), paste into panel, open a NEW access link.';
  } else {
    v.className = 'verdict dead';
    v.textContent = 'DEAD / NO AUTH — GetStore and research both failing. Refresh Zik cookies in panel, then new access link.';
  }
}

async function runProbe(){
  const btn = document.getElementById('btnProbe');
  btn.disabled = true;
  document.getElementById('probeOut').textContent = 'Probing api.zikanalytics.com …';
  try{
    const r = await fetch('/__debug.json?probe=1&t='+Date.now());
    const d = await r.json();
    lastSession = d.session || lastSession;
    lastProbe = d.probe;
    showProbe(d.probe || {error:'no probe'});
    await loadLogs();
  }catch(e){
    document.getElementById('probeOut').textContent = 'probe failed: '+e;
  }
  btn.disabled = false;
}

async function loadLogs(){
  try{
    const r = await fetch('/__logs.json?t='+Date.now());
    const data = await r.json();
    lastLogs = (data.logs||[]).filter(e => e.status>=400 || e.source==='CLIENT' || e.source==='NET' || (e.detail||'').indexOf('401')>=0 || (e.detail||'').indexOf('cookie-only')>=0).slice(-120).reverse();
    document.getElementById('rows').innerHTML = lastLogs.map(e=>{
      const cls = e.status===401||e.status===403?'s401':'';
      return '<tr><td>'+new Date(e.time).toLocaleTimeString()+'</td><td>'+esc(e.source)+'</td><td class="'+cls+'">'+(e.status||'')+'</td><td>'+esc(e.method)+'</td><td>'+esc(e.path)+'</td><td class="detail">'+esc(e.detail)+'</td></tr>';
    }).join('') || '<tr><td colspan="6">No error rows yet — use the tool, then refresh.</td></tr>';
  }catch(e){
    document.getElementById('rows').innerHTML = '<tr><td colspan="6">logs failed: '+esc(e)+'</td></tr>';
  }
}

async function copyDump(){
  const dump = {
    when: new Date().toISOString(),
    host: location.host,
    session: lastSession,
    probe: lastProbe,
    errors: lastLogs.slice(0, 80),
  };
  const text = JSON.stringify(dump, null, 2);
  try {
    await navigator.clipboard.writeText(text);
    alert('Dump copied — paste it in chat.');
  } catch (e) {
    prompt('Copy this dump:', text);
  }
}

loadSession();
loadLogs();
setInterval(loadLogs, 4000);
</script></div></body></html>`)
}

func zikDebugJSONHandler(w http.ResponseWriter, r *http.Request) {
	cfg := loadConfig()
	out := map[string]interface{}{
		"time": time.Now().UTC().Format(time.RFC3339),
	}
	sess := map[string]interface{}{
		"user":           "",
		"account":        "",
		"account_id":     0,
		"bearer":         false,
		"session_cookie": false,
		"jwt_expired":    false,
		"has_proxy":      false,
		"claims":         "",
		"cookie_bytes":   0,
	}
	if u, err := panelSessionUsername(r); err == nil {
		sess["user"] = u
		if tok, _, ok := sessionFromRequest(r); ok {
			if acc, aerr := loadPanelSessionAccount(cfg, tok); aerr == nil {
				sess["account"] = acc.Name
				sess["account_id"] = acc.ID
				sess["has_proxy"] = strings.TrimSpace(acc.Proxy) != ""
				sess["cookie_bytes"] = len(strings.TrimSpace(acc.Cookie))
				bOK, sOK, exp := zikAuthDiag(acc.Cookie)
				sess["bearer"] = bOK
				sess["session_cookie"] = sOK
				sess["jwt_expired"] = exp
				sess["claims"] = zikJWTClaimsSummary(acc.Cookie)
				if r.URL.Query().Get("probe") == "1" || r.URL.Query().Get("probe") == "true" {
					basicOK, researchOK, detail := zikProbeAccount(acc)
					out["probe"] = map[string]interface{}{
						"basic_ok":    basicOK,
						"research_ok": researchOK,
						"detail":      detail,
						"account":     acc.Name,
						"account_id":  acc.ID,
					}
					pushProxyLog(ProxyLogEntry{
						Source: "PROBE", Level: "info", Method: "GET", Path: "/__debug", Status: 200,
						User: u, Account: acc.Name,
						Detail: fmt.Sprintf("basic=%v research=%v %s", basicOK, researchOK, detail),
					})
				}
			} else {
				sess["account"] = "load_error: " + aerr.Error()
			}
		}
	} else {
		sess["user"] = "not_logged_in — open via panel access link first"
	}
	out["session"] = sess
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(out)
}

func zikDebugButtonScript() string {
	return `<script data-tm-zik-debug="1">
(function(){
  if (window.__zikDebugBtn) return;
  window.__zikDebugBtn = true;
  function boot(){
    if (!document.body || document.getElementById('tm-zik-debug')) return;
    var a = document.createElement('a');
    a.id = 'tm-zik-debug';
    a.href = '/__debug';
    a.target = '_blank';
    a.rel = 'noopener';
    a.textContent = 'Debug';
    a.title = 'Zik debug desk — probe + copy error dump';
    a.style.cssText = 'position:fixed;bottom:20px;left:20px;z-index:999999;background:#0f172a;color:#e2e8f0;border:1px solid #334155;border-radius:12px;padding:10px 14px;font:600 13px/1 system-ui,sans-serif;text-decoration:none;box-shadow:0 8px 24px rgba(0,0,0,.35)';
    document.body.appendChild(a);
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot);
  else boot();
})();
</script>`
}
