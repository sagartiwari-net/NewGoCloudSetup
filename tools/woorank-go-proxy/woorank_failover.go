package main

import (
	"html"
	"log"
	"net/http"
	"strings"
)

func woorankLoggedOutHTML(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	return strings.Contains(strings.ToLower(string(body)), "you are not authorized to access this page")
}

const woorankSwitchRetryScript = `<script>
(function () {
  var n = 0;
  try { n = parseInt(sessionStorage.getItem("tm_acct_try") || "0", 10) || 0; } catch (e) {}
  n += 1;
  try { sessionStorage.setItem("tm_acct_try", String(n)); } catch (e) {}
  if (n >= 3) {
    var title = document.querySelector("h1");
    var msg = document.querySelector(".msg");
    if (title) title.textContent = "Temporarily Unavailable";
    if (msg) msg.textContent = "WooRank is temporarily unavailable. Please try again in a few minutes.";
    var pill = document.querySelector(".pill");
    if (pill) pill.style.display = "none";
    var foot = document.querySelector(".foot");
    if (foot) foot.textContent = "Open this tool again from your access link";
    var ring = document.querySelector(".ring");
    if (ring) ring.classList.remove("spin");
    try { sessionStorage.removeItem("tm_acct_try"); } catch (e) {}
    return;
  }
  setTimeout(function () { window.location.reload(); }, 1200);
})();
</script>`

func renderWoorankSwitchPage(w http.ResponseWriter, accountName string) {
	message := "Account logged out. Switching to another account..."
	if accountName != "" {
		message = "Account logged out. Switching to " + html.EscapeString(accountName) + "..."
	}
	writeLightCard(w, http.StatusOK, lightCard{
		Title:       "Switching account",
		Heading:     "Switching account",
		Message:     message,
		Badge:       "Trying one account at a time",
		Footer:      "This keeps trying until an account is available",
		Spin:        true,
		ExtraScript: woorankSwitchRetryScript,
	})
}

func renderWoorankUnavailablePage(w http.ResponseWriter) {
	writeLightCard(w, http.StatusServiceUnavailable, lightCard{
		Title:   "Temporarily Unavailable",
		Heading: "Temporarily Unavailable",
		Message: "WooRank is temporarily unavailable. Please try again in a few minutes.",
		Footer:  "Open this tool again from your access link",
	})
}

// serveWoorankLogoutSwitch replaces the logged-out WooRank page with the light switch card.
func serveWoorankLogoutSwitch(w http.ResponseWriter, cfg Config, sessionToken, currentUser string, activeAcc ToolAccount) {
	reason := "html_logout:not_authorized"
	log.Printf("[FAILOVER] WooRank logged out user=%s account=%s (ID:%d)", currentUser, activeAcc.Name, activeAcc.ID)
	next, err := panelSwitchAccount(cfg, sessionToken, activeAcc.ID, activeAcc.Name, currentUser, reason)
	if err != nil || next.ID == 0 || next.ID == activeAcc.ID {
		log.Printf("[FAILOVER] no other WooRank account user=%s err=%v", currentUser, err)
		renderWoorankUnavailablePage(w)
		return
	}
	renderWoorankSwitchPage(w, next.Name)
}

func woorankLoginWallWatchScript() string {
	return `<script>
(function () {
  if (window.__tmWrankWatch) return;
  window.__tmWrankWatch = true;
  var started = false;
  function wall() {
    var text = "";
    try { text = (document.body && (document.body.innerText || document.body.textContent) || "").toLowerCase(); } catch (e) {}
    return text.indexOf("you are not authorized to access this page") !== -1;
  }
  function arm() {
    if (started || !wall()) return;
    started = true;
    var n = 0;
    try { n = parseInt(sessionStorage.getItem("tm_acct_try") || "0", 10) || 0; } catch (e) {}
    if (n >= 3) return;
    fetch("/api/rotate-session?reason=client_not_authorized", { credentials: "include", cache: "no-store" })
      .finally(function () { window.location.reload(); });
  }
  setTimeout(function () {
    if (!wall()) { try { sessionStorage.removeItem("tm_acct_try"); } catch (e) {} }
  }, 8000);
  setTimeout(arm, 1200);
  var ticks = 0;
  var timer = setInterval(function () {
    ticks += 1;
    if (started || ticks > 8) { clearInterval(timer); return; }
    arm();
  }, 1000);
})();
</script>`
}
