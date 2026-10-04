package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type creditUserKey struct{}

func isBillableCompletion(r *http.Request) bool {
	if r == nil || r.Method != http.MethodPost {
		return false
	}
	path := r.URL.Path
	return strings.Contains(path, "/chat_conversations/") && strings.HasSuffix(path, "/completion")
}

type panelCreditState struct {
	websiteID int
	userID    int
	limit     int
	used      int
	resetDays int
	blocked   bool
}

func panelCreditStateFor(username string) (panelCreditState, error) {
	state := panelCreditState{resetDays: 1, limit: 100}
	cfg := loadConfig()
	db, err := openPanelDB(cfg)
	if err != nil {
		return state, err
	}
	var rawDefaults string
	if err := db.QueryRow(`SELECT id, COALESCE(default_limits_json, '{}') FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&state.websiteID, &rawDefaults); err != nil {
		return state, err
	}
	defaults := map[string]int{}
	_ = json.Unmarshal([]byte(rawDefaults), &defaults)
	if n, ok := defaults["credits"]; ok {
		state.limit = n
	}
	var status, expire string
	var meterID, meterLimit, meterUsed, resetDays, custom int
	err = db.QueryRow(`SELECT u.id, u.status, COALESCE(u.custom_limit_expire_at, ''),
		COALESCE(m.id, 0), COALESCE(m."limit", 0), COALESCE(m.used, 0), COALESCE(m.reset_days, 1), COALESCE(m.is_custom, 0)
		FROM panel_users u
		LEFT JOIN user_meters m ON m.user_id=u.id AND m.key='credits'
		WHERE u.website_id=? AND u.username=?`, state.websiteID, username).Scan(&state.userID, &status, &expire, &meterID, &meterLimit, &meterUsed, &resetDays, &custom)
	if err == nil {
		if status == "suspended" {
			state.blocked = true
		}
		if resetDays > 0 {
			state.resetDays = resetDays
		}
		customActive := custom == 1
		if expire != "" && creditExpirePassed(expire) {
			customActive = false
		}
		state.used = meterUsed
		if meterID > 0 {
			state.limit = meterLimit
		} else if customActive {
			state.limit = meterLimit
		}
	}
	return state, nil
}

func creditLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return time.FixedZone("IST", 5*3600+30*60)
	}
	return loc
}

func creditExpirePassed(raw string) bool {
	raw = strings.TrimSpace(raw)
	loc := creditLocation()
	var day time.Time
	if t, err := time.ParseInLocation("2006-01-02", raw, loc); err == nil {
		day = t
	} else if t, err := time.Parse(time.RFC3339, raw); err == nil {
		t = t.In(loc)
		day = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	} else {
		return false
	}
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return today.After(day)
}

func panelCreditsAllow(username string) bool {
	state, err := panelCreditStateFor(username)
	if err != nil {
		log.Printf("[CREDIT] check failed user=%s: %v", username, err)
		return true
	}
	if state.blocked || state.limit == 0 {
		return false
	}
	if state.limit < 0 {
		return true
	}
	return state.used < state.limit
}

func panelCreditsCharge(username, path string) {
	state, err := panelCreditStateFor(username)
	if err != nil || state.websiteID == 0 {
		log.Printf("[CREDIT] charge skipped user=%s: %v", username, err)
		return
	}
	db, err := openPanelDB(loadConfig())
	if err != nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO usage_events (website_id, username, limit_key, limit_label, reset_days, action, target_path, amount, timestamp)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		state.websiteID, username, "credits", "Messages", state.resetDays, "message", path, 1, now)
	if err != nil {
		log.Printf("[CREDIT] usage insert failed user=%s: %v", username, err)
		return
	}
	if state.userID == 0 {
		res, err := db.Exec(`INSERT INTO panel_users (website_id, username, status, custom_limit_expire_at) VALUES (?,?,?,NULL)`,
			state.websiteID, username, "active")
		if err != nil {
			log.Printf("[CREDIT] user create failed user=%s: %v", username, err)
			return
		}
		id, _ := res.LastInsertId()
		state.userID = int(id)
		_, _ = db.Exec(`INSERT INTO user_meters (user_id, key, label, reset_days, used, "limit", is_custom) VALUES (?,?,?,?,?,?,0)`,
			state.userID, "credits", "Messages", state.resetDays, state.used+1, state.limit)
	} else {
		res, err := db.Exec(`UPDATE user_meters SET used = used + 1 WHERE user_id=? AND key='credits'`, state.userID)
		changed := int64(0)
		if err == nil && res != nil {
			changed, _ = res.RowsAffected()
		}
		if err != nil || changed == 0 {
			_, _ = db.Exec(`INSERT INTO user_meters (user_id, key, label, reset_days, used, "limit", is_custom) VALUES (?,?,?,?,?,?,0)`,
				state.userID, "credits", "Messages", state.resetDays, state.used+1, state.limit)
		}
	}
	log.Printf("[CREDIT] 1 message user=%s used=%d limit=%d", username, state.used+1, state.limit)
}

func panelAssignedShowLimit(r *http.Request, cfg Config, username string) bool {
	db, err := openPanelDB(cfg)
	if err != nil {
		return false
	}
	var websiteID, accountID int
	if err = db.QueryRow(`SELECT id FROM websites WHERE domain=?`, cfg.PublicHost).Scan(&websiteID); err != nil || websiteID == 0 {
		return false
	}
	if username != "" {
		var mode string
		_ = db.QueryRow(`SELECT COALESCE(limit_visibility, '') FROM panel_users WHERE website_id=? AND username=?`, websiteID, username).Scan(&mode)
		switch strings.ToLower(strings.TrimSpace(mode)) {
		case "show":
			return true
		case "hide":
			return false
		}
	}
	token := ""
	if c, err := r.Cookie("ct_session"); err == nil {
		token = c.Value
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if token != "" {
		_ = db.QueryRow(`SELECT assigned_account_id FROM live_sessions WHERE session_token=? AND website_id=? AND expires_at>?`, token, websiteID, now).Scan(&accountID)
	}
	if accountID == 0 && username != "" {
		_ = db.QueryRow(`SELECT assigned_account_id FROM live_sessions WHERE username=? AND website_id=? AND expires_at>? ORDER BY id DESC LIMIT 1`, username, websiteID, now).Scan(&accountID)
	}
	if accountID == 0 {
		return false
	}
	var show int
	if err = db.QueryRow(`SELECT COALESCE(show_limit, 0) FROM accounts WHERE id=?`, accountID).Scan(&show); err != nil {
		return false
	}
	return show == 1
}

func rejectIfChatCreditSpent(w http.ResponseWriter, r *http.Request, cfg Config, username string) bool {
	if !usesPanelAccountMode(cfg) || !isBillableCompletion(r) || strings.TrimSpace(username) == "" {
		return false
	}
	if panelCreditsAllow(username) {
		return false
	}
	log.Printf("[CREDIT] blocked user=%s path=%s", username, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-TM-Limit", "credit")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusForbidden)
	fmt.Fprint(w, `{"error":{"message":"Daily limit reached","code":"limit_reached","label":"Messages"}}`)
	return true
}

func limitWidgetScript(toolName string) string {
	name := strings.TrimSpace(toolName)
	if name == "" {
		name = "Claude AI"
	}
	return fmt.Sprintf(`<script data-tm-credits="1">
(function() {
    var CREDIT_LABEL = 'Messages';
    var TOOL_NAME = %q;
    var open = false;
    var latest = null;
    function removeDock() {
        var el = document.getElementById('tm-limit-dock');
        if (el) el.remove();
    }
    function host() {
        if (latest && !latest.show_limit) { removeDock(); return null; }
        var el = document.getElementById('tm-limit-dock');
        if (el) return el;
        el = document.createElement('div');
        el.id = 'tm-limit-dock';
        var root = el.attachShadow({ mode: 'open' });
        root.innerHTML = ''
            + '<style>'
            + ':host{all:initial}'
            + '.tab{position:fixed;left:16px;bottom:24px;z-index:2147483646;width:48px;height:48px;border:0;border-radius:999px;background:#22c55e;color:#fff;display:grid;place-items:center;box-shadow:0 10px 24px rgba(22,163,74,.35);cursor:pointer}'
            + '.tab svg{width:22px;height:22px;display:block}'
            + '.tab.warn{background:#f59e0b}.tab.low{background:#ef4444}.tab.hide{opacity:0;pointer-events:none}'
            + '.back{position:fixed;inset:0;z-index:2147483646;background:rgba(15,23,42,.18);opacity:0;pointer-events:none;transition:opacity .2s}'
            + '.back.show{opacity:1;pointer-events:auto}'
            + '.card{position:fixed;left:16px;bottom:24px;z-index:2147483647;width:232px;background:#f3fbf6;color:#14532d;border-radius:28px;box-shadow:0 22px 50px rgba(15,23,42,.18);padding:16px 16px 14px;box-sizing:border-box;font:500 14px/1.3 system-ui,sans-serif;transform:translateY(10px) scale(.96);opacity:0;pointer-events:none;transition:transform .22s ease,opacity .22s ease}'
            + '.card.show{transform:none;opacity:1;pointer-events:auto}'
            + '.head{display:flex;align-items:center;justify-content:space-between;margin-bottom:6px}'
            + '.title{font-weight:750;font-size:15px}'
            + '.x{border:0;background:#e7f6ec;color:#166534;width:28px;height:28px;border-radius:999px;cursor:pointer;font:700 16px/1 system-ui,sans-serif}'
            + '.ringwrap{position:relative;width:168px;height:168px;margin:4px auto 8px}'
            + '.ring{width:168px;height:168px;transform:rotate(-90deg)}'
            + '.track{fill:none;stroke:#d9f3e3;stroke-width:10}'
            + '.fill{fill:none;stroke:#22c55e;stroke-width:10;stroke-linecap:round;stroke-dasharray:289;stroke-dashoffset:0;transition:stroke-dashoffset .35s ease,stroke .2s}'
            + '.fill.warn{stroke:#f59e0b}.fill.low{stroke:#ef4444}'
            + '.center{position:absolute;inset:0;display:flex;flex-direction:column;align-items:center;justify-content:center}'
            + '.num{font-size:40px;font-weight:800;letter-spacing:-.04em;color:#14532d}'
            + '.sub{margin-top:2px;color:#4d7c5e;font-size:13px}'
            + '.meter{background:#fff;border-radius:16px;padding:10px 12px}'
            + '.meter-top{display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;color:#166534;font-size:13px}'
            + '.count{font-weight:750}'
            + '.bar{height:6px;border-radius:999px;background:#e7f6ec;overflow:hidden}'
            + '.barfill{height:100%%;width:0;border-radius:999px;background:#22c55e;transition:width .35s ease}'
            + '.barfill.warn{background:#f59e0b}.barfill.low{background:#ef4444}'
            + '<' + '/style>'
            + '<button class="tab" id="tm-tab" type="button" aria-label="Messages"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="8" fill="none" stroke="currentColor" stroke-width="2" opacity=".35"><' + '/circle><path d="M12 4a8 8 0 0 1 8 8" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><' + '/path><' + '/svg><' + '/button>'
            + '<div class="back" id="tm-back"><' + '/div>'
            + '<aside class="card" id="tm-drawer">'
            + '<div class="head"><div class="title" id="tm-title">Messages<' + '/div><button class="x" id="tm-close" type="button" aria-label="Close">×<' + '/button><' + '/div>'
            + '<div class="ringwrap"><svg class="ring" viewBox="0 0 120 120"><circle class="track" cx="60" cy="60" r="46"><' + '/circle><circle class="fill" id="tm-ring" cx="60" cy="60" r="46"><' + '/circle><' + '/svg>'
            + '<div class="center"><div class="num" id="tm-left">0<' + '/div><div class="sub" id="tm-sub">left<' + '/div><' + '/div><' + '/div>'
            + '<div class="meter"><div class="meter-top"><span id="tm-meter-label">Messages<' + '/span><span class="count" id="tm-count">0 / 0<' + '/span><' + '/div><div class="bar"><div class="barfill" id="tm-bar"><' + '/div><' + '/div><' + '/div>'
            + '<' + '/aside>';
        (document.body || document.documentElement).appendChild(el);
        var shadow = root;
        shadow.getElementById('tm-tab').addEventListener('click', function(ev){ ev.stopPropagation(); setOpen(true); });
        shadow.getElementById('tm-close').addEventListener('click', function(ev){ ev.stopPropagation(); setOpen(false); });
        shadow.getElementById('tm-back').addEventListener('click', function(){ setOpen(false); });
        shadow.getElementById('tm-drawer').addEventListener('click', function(ev){ ev.stopPropagation(); });
        return el;
    }
    function setOpen(next) {
        open = next;
        var el = host();
        if (!el) return;
        var root = el.shadowRoot;
        root.getElementById('tm-drawer').classList.toggle('show', open);
        root.getElementById('tm-back').classList.toggle('show', open);
        root.getElementById('tm-tab').classList.toggle('hide', open);
    }
    function paint(d) {
        if (!d || !d.show_limit) { removeDock(); return; }
        var el = host();
        if (!el) return;
        var root = el.shadowRoot;
        root.getElementById('tm-title').textContent = CREDIT_LABEL;
        root.getElementById('tm-meter-label').textContent = CREDIT_LABEL;
        var tab = root.getElementById('tm-tab');
        var ring = root.getElementById('tm-ring');
        var bar = root.getElementById('tm-bar');
        var used = Number(d.credit_used) || 0;
        var limit = Number(d.credit_limit);
        var unlimited = !(limit >= 0);
        var left = unlimited ? used : Math.max(0, limit - used);
        var ratio = unlimited ? 1 : (limit === 0 ? 0 : left / limit);
        root.getElementById('tm-left').textContent = unlimited ? '∞' : String(left);
        root.getElementById('tm-sub').textContent = unlimited ? 'unlimited' : 'left';
        root.getElementById('tm-count').textContent = unlimited ? (String(used) + ' used') : (String(used) + ' / ' + String(limit));
        ring.style.strokeDasharray = '289';
        ring.style.strokeDashoffset = String(289 * (1 - ratio));
        bar.style.width = Math.round(ratio * 100) + '%%';
        var low = !unlimited && left < 3;
        var warn = !unlimited && left >= 3 && left < 10;
        tab.classList.toggle('low', low);
        tab.classList.toggle('warn', warn);
        ring.classList.toggle('low', low);
        ring.classList.toggle('warn', warn);
        bar.classList.toggle('low', low);
        bar.classList.toggle('warn', warn);
    }
    function showLimitCard() {
        if (document.getElementById('tm-limit-screen')) return;
        var el = document.createElement('div');
        el.id = 'tm-limit-screen';
        el.style.cssText = 'position:fixed;inset:0;z-index:2147483647;background:#eef3f8;color:#0f172a;display:flex;align-items:center;justify-content:center;padding:24px;font-family:system-ui,-apple-system,Segoe UI,sans-serif;';
        el.innerHTML = '<div style="width:min(440px,92vw);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px 36px;text-align:center;">'
            + '<div style="width:78px;height:78px;margin:0 auto 22px;border-radius:50%%;background:#fff;display:grid;place-items:center;font-size:28px;border:1px solid #e6ebf2;">&#128274;<' + '/div>'
            + '<h1 style="font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em;margin:0 0 12px;">Daily Limit Reached<' + '/h1>'
            + '<p style="color:#64748b;font-size:15px;line-height:1.55;margin:0;">You have used the daily ' + CREDIT_LABEL + ' limit for ' + TOOL_NAME + '. The limit resets at midnight (12:00 AM IST).<' + '/p>'
            + '<p style="margin:22px 0 0;color:#94a3b8;font-size:13px;">This limit applies to the current access<' + '/p><' + '/div>';
        (document.body || document.documentElement).appendChild(el);
    }
    function noCreditsLeft() {
        if (!latest || !latest.show_limit) return false;
        var limit = Number(latest.credit_limit);
        if (!(limit >= 0)) return false;
        var used = Number(latest.credit_used) || 0;
        return used >= limit;
    }
    function isChatSend(url) {
        var target = String(url || '');
        return target.indexOf('/chat_conversations/') !== -1 && target.indexOf('/completion') !== -1;
    }
    function blockedChatResponse() {
        showLimitCard();
        return new Response('{"error":{"message":"Daily limit reached","code":"limit_reached"}}', {
            status: 403,
            headers: { 'Content-Type': 'application/json', 'X-TM-Limit': 'credit' }
        });
    }
    function updateBadge() {
        window.fetch('/api/user-limits', { credentials: 'same-origin', cache: 'no-store' })
            .then(function(r){ return r.json(); })
            .then(function(d){ latest = d; paint(d); })
            .catch(function(){});
    }
    window.__tmUpdateCredits = updateBadge;
    function boot() { updateBadge(); }
    if (document.body) boot();
    else document.addEventListener('DOMContentLoaded', boot);
    setInterval(function(){ updateBadge(); }, 4000);
    var orig = window.fetch;
    window.fetch = function(input, init) {
        var url = typeof input === 'string' ? input : (input && input.url) || '';
        var target = String(url);
        if (noCreditsLeft() && isChatSend(target)) return Promise.resolve(blockedChatResponse());
        var done = orig.apply(this, arguments);
        if (isChatSend(target)) {
            return done.then(function(res) {
                if (res && res.headers && res.headers.get('X-TM-Limit')) {
                    showLimitCard();
                    return new Response('{"error":"limit_reached"}', { status: 403, headers: { 'Content-Type': 'application/json' } });
                }
                if (res && res.ok) setTimeout(updateBadge, 400);
                return res;
            });
        }
        return done;
    };
})();
</script>`, name)
}
