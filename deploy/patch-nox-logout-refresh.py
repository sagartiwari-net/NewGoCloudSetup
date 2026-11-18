#!/usr/bin/env python3
"""Patch Nox nxsm/nxaud logout + 3s retry pages."""
from pathlib import Path

BASE = Path(__file__).resolve().parents[1] / "tools/recloud/noxtools"
TOOLS = [f"nxsm{i}" for i in range(1, 6)] + [f"nxaud{i}" for i in range(1, 4)]

LOGOUT_OLD = """\tlog.Printf(\"[LB] logout recorded account=%s user=%s reason=%s\", acc.Name, username, reason)
\t// Mark account logged_out when session died and nothing else to switch to — future automation picks this up.
\tif reason == \"session_expired\" && !switched && acc.ID > 0 {
\t\t_, _ = db.Exec(`UPDATE accounts SET status='logged_out' WHERE id=?`, acc.ID)
\t}
\treturn switched, nextName
"""

LOGOUT_NEW = """\tlog.Printf(\"[LB] logout recorded account=%s user=%s reason=%s\", acc.Name, username, reason)
\t// Do NOT mark the account logged_out here. That flips the tool to
\t// Temporarily Unavailable (no retry). Keep status active so the browser
\t// stays on \"Signing you in\" and reloads every 3s while automation
\t// refreshes the cookie (and Shared-cookies followers).
\treturn switched, nextName
"""

SCRIPT_OLD = """func loggedOutRefreshScript() string {
\treturn `<script>(function(){var k=\"oneclickgo-relogin\",n=0;try{n=Number(sessionStorage.getItem(k)||\"0\")}catch(e){}if(n>=8)return;try{sessionStorage.setItem(k,String(n+1))}catch(e){}setTimeout(function(){location.reload()},n===0?3000:4000)})();</script>`
}"""

SCRIPT_NEW = """func loggedOutRefreshScript() string {
\t// Fixed 3s between tries. Cap retries while automation refreshes the cookie.
\treturn `<script>(function(){var k=\"oneclickgo-relogin\",n=0;try{n=Number(sessionStorage.getItem(k)||\"0\")}catch(e){}if(!Number.isFinite(n)||n<0)n=0;if(n>=40)return;try{sessionStorage.setItem(k,String(n+1))}catch(e){}setTimeout(function(){location.reload()},3000)})();</script>`
}"""

UNAVAIL_OLD = """func renderNoActiveAccountsPage(w http.ResponseWriter, cfg Config) {
\twriteLightCard(w, http.StatusServiceUnavailable, lightCard{
\t\tTitle:   \"Temporarily Unavailable\",
\t\tHeading: \"Temporarily Unavailable\",
\t\tMessage: \"Tools is temporarily unavailable. Please try again in a few minutes.\",
\t\tFooter:  \"Open this tool again from your access link\",
\t})
}"""

UNAVAIL_NEW = """func renderNoActiveAccountsPage(w http.ResponseWriter, cfg Config) {
\twriteLightCard(w, http.StatusServiceUnavailable, lightCard{
\t\tTitle:       \"Temporarily Unavailable\",
\t\tHeading:     \"Temporarily Unavailable\",
\t\tMessage:     \"Tools is temporarily unavailable. Please try again in a few minutes.\",
\t\tFooter:      \"This page checks again every 3 seconds\",
\t\tSpin:        true,
\t\tExtraScript: loggedOutRefreshScript(),
\t})
}"""


def main() -> None:
    for name in TOOLS:
        logout = BASE / name / "logout.go"
        panel = BASE / name / "panel.go"
        lt = logout.read_text()
        if LOGOUT_OLD not in lt:
            raise SystemExit(f"logout pattern missing: {logout}")
        logout.write_text(lt.replace(LOGOUT_OLD, LOGOUT_NEW, 1))
        pt = panel.read_text()
        if SCRIPT_OLD not in pt:
            raise SystemExit(f"script pattern missing: {panel}")
        if UNAVAIL_OLD not in pt:
            raise SystemExit(f"unavail pattern missing: {panel}")
        pt = pt.replace(SCRIPT_OLD, SCRIPT_NEW, 1).replace(UNAVAIL_OLD, UNAVAIL_NEW, 1)
        panel.write_text(pt)
        print("patched", name)


if __name__ == "__main__":
    main()
