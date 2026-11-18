#!/usr/bin/env python3
"""Update Nox logged-out / unavailable pages to show Updating tools + 3-2-1 countdown."""
from pathlib import Path

BASE = Path(__file__).resolve().parents[1] / "tools/recloud/noxtools"
TOOLS = [f"nxsm{i}" for i in range(1, 6)] + [f"nxaud{i}" for i in range(1, 4)]

REPLACEMENTS = [
(
"""func renderLoggedOutPage(w http.ResponseWriter, cfg Config) {
\twriteLightCard(w, http.StatusForbidden, lightCard{
\t\tTitle:       \"Logged out\",
\t\tHeading:     \"Signing you in\",
\t\tMessage:     \"Tools signed out. Signing you in again.\",
\t\tFooter:      \"This page checks again in a few seconds\",
\t\tExtraScript: loggedOutRefreshScript(),
\t})
}""",
"""func renderLoggedOutPage(w http.ResponseWriter, cfg Config) {
\twriteLightCard(w, http.StatusForbidden, lightCard{
\t\tTitle:       \"Updating tools\",
\t\tHeading:     \"Updating tools\",
\t\tMessage:     \"Tools signed out. Updating tools — retrying automatically.\",
\t\tFooter:      \"Refreshing when the countdown reaches 0\",
\t\tSpin:        true,
\t\tCountdown:   true,
\t\tExtraScript: loggedOutRefreshScript(),
\t})
}""",
),
(
"""func loggedOutRefreshScript() string {
\t// Fixed 3s between tries. Cap retries while automation refreshes the cookie.
\treturn `<script>(function(){var k=\"oneclickgo-relogin\",n=0;try{n=Number(sessionStorage.getItem(k)||\"0\")}catch(e){}if(!Number.isFinite(n)||n<0)n=0;if(n>=40)return;try{sessionStorage.setItem(k,String(n+1))}catch(e){}setTimeout(function(){location.reload()},3000)})();</script>`
}""",
"""func loggedOutRefreshScript() string {
\t// Visible 3-2-1 countdown, then reload. Cap retries while automation refreshes the cookie.
\treturn `<script>(function(){var k=\"oneclickgo-relogin\",n=0;try{n=Number(sessionStorage.getItem(k)||\"0\")}catch(e){}if(!Number.isFinite(n)||n<0)n=0;if(n>=40)return;try{sessionStorage.setItem(k,String(n+1))}catch(e){}var left=3;var el=document.getElementById(\"tm-countdown\");var label=document.getElementById(\"tm-countdown-label\");function paint(){if(el)el.textContent=String(left);if(label)label.textContent=left<=0?\"Refreshing now…\":\"Updating tools in \"+left+\" sec\";}paint();var t=setInterval(function(){left-=1;if(left<=0){clearInterval(t);paint();location.reload();return;}paint();},1000);})();</script>`
}""",
),
(
"""func renderNoActiveAccountsPage(w http.ResponseWriter, cfg Config) {
\twriteLightCard(w, http.StatusServiceUnavailable, lightCard{
\t\tTitle:       \"Temporarily Unavailable\",
\t\tHeading:     \"Temporarily Unavailable\",
\t\tMessage:     \"Tools is temporarily unavailable. Please try again in a few minutes.\",
\t\tFooter:      \"This page checks again every 3 seconds\",
\t\tSpin:        true,
\t\tExtraScript: loggedOutRefreshScript(),
\t})
}""",
"""func renderNoActiveAccountsPage(w http.ResponseWriter, cfg Config) {
\twriteLightCard(w, http.StatusServiceUnavailable, lightCard{
\t\tTitle:       \"Updating tools\",
\t\tHeading:     \"Updating tools\",
\t\tMessage:     \"Tools is refreshing. Please wait — retrying automatically.\",
\t\tFooter:      \"Refreshing when the countdown reaches 0\",
\t\tSpin:        true,
\t\tCountdown:   true,
\t\tExtraScript: loggedOutRefreshScript(),
\t})
}""",
),
(
"""type lightCard struct {
\tTitle       string
\tHeading     string
\tMessage     string
\tBadge       string
\tFooter      string
\tSpin        bool
\tExtraScript string
}""",
"""type lightCard struct {
\tTitle       string
\tHeading     string
\tMessage     string
\tBadge       string
\tFooter      string
\tSpin        bool
\tCountdown   bool
\tExtraScript string
}""",
),
(
"""\tfooter := \"\"
\tif card.Footer != \"\" {
\t\tfooter = `<p class=\"foot\">` + html.EscapeString(card.Footer) + `</p>`
\t}
""",
"""\tfooter := \"\"
\tif card.Footer != \"\" {
\t\tfooter = `<p class=\"foot\">` + html.EscapeString(card.Footer) + `</p>`
\t}
\tcountdown := \"\"
\tif card.Countdown {
\t\tcountdown = `<div class=\"count-wrap\"><div class=\"count\" id=\"tm-countdown\">3</div><p class=\"count-label\" id=\"tm-countdown-label\">Updating tools in 3 sec</p></div>`
\t}
""",
),
(
""".foot { margin-top:18px;color:#94a3b8;font-size:13px; }
@keyframes spin { to { transform:rotate(360deg); } }""",
""".foot { margin-top:18px;color:#94a3b8;font-size:13px; }
.count-wrap { margin-top:22px; }
.count { font-size:42px;font-weight:800;letter-spacing:-.04em;color:#2563eb;line-height:1; }
.count-label { margin-top:8px;color:#64748b;font-size:14px;font-weight:600; }
@keyframes spin { to { transform:rotate(360deg); } }""",
),
(
"""<h1>%s</h1>
<p class=\"msg\">%s</p>
%s
%s
</div>
%s
</body>
</html>`, html.EscapeString(card.Title), spinClass, html.EscapeString(card.Heading), card.Message, badge, footer, card.ExtraScript)""",
"""<h1>%s</h1>
<p class=\"msg\">%s</p>
%s
%s
%s
</div>
%s
</body>
</html>`, html.EscapeString(card.Title), spinClass, html.EscapeString(card.Heading), card.Message, badge, countdown, footer, card.ExtraScript)""",
),
]


def main() -> None:
    for name in TOOLS:
        panel = BASE / name / "panel.go"
        t = panel.read_text()
        for old, new in REPLACEMENTS:
            if old not in t:
                raise SystemExit(f"{name}: missing block:\n{old[:120]}")
            t = t.replace(old, new, 1)
        panel.write_text(t)
        print("patched", name)


if __name__ == "__main__":
    main()
