package main

import (
	"bytes"
	"html"
	"net/http"
	"strings"
)

// productButton is one launcher button. To add another product later, append one
// entry here. Path is a path on this tool, and that button opens only that path.
type productButton struct {
	Label string
	Path  string
}

var productButtons = []productButton{
	{Label: "Freedom Ticket 4.0", Path: "/products/freedom-ticket-4-0"},
	{Label: "Freedom Ticket 3.0", Path: "/products/freedom-ticket-3-0"},
	{Label: "Exit Ticket", Path: "/products/exit-ticket"},
	{Label: "Ads Academy", Path: "/products/ads-academy"},
}

func renderProductLauncher(w http.ResponseWriter, cfg Config) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(renderProductLauncherHTML(cfg)))
}

func renderProductLauncherHTML(cfg Config) string {
	name := html.EscapeString(toolDisplayName(cfg))
	var links strings.Builder
	for _, btn := range productButtons {
		if !validProductPath(btn.Path) {
			continue
		}
		links.WriteString(`<a class="product" href="`)
		links.WriteString(html.EscapeString(btn.Path))
		links.WriteString(`">`)
		links.WriteString(html.EscapeString(btn.Label))
		links.WriteString(`</a>`)
	}
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0">
<title>` + name + `</title>
<style>
* { box-sizing:border-box;margin:0;padding:0; }
body { min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;background:#eef3f8;color:#0f172a;font-family:system-ui,-apple-system,Segoe UI,sans-serif; }
.card { width:min(520px,100%);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:40px 32px 32px; }
h1 { font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em; }
.brand { color:#2563eb; }
.msg { margin-top:8px;color:#64748b;font-size:15px;line-height:1.5; }
.products { display:grid;gap:10px;margin-top:24px; }
.product { display:flex;align-items:center;justify-content:center;min-height:48px;padding:12px 16px;border-radius:14px;background:#0f172a;color:#fff;font-weight:700;text-decoration:none; }
.product:hover { background:#1e293b; }
</style>
</head>
<body>
<main class="card">
<h1 class="brand">` + name + `</h1>
<p class="msg">Open a product. Each button stays on this tool.</p>
<nav class="products" data-tm-products>` + links.String() + `</nav>
</main>
</body>
</html>`
}

func injectProductSwitcher(body []byte, currentPath string) []byte {
	if len(body) == 0 || bytes.Contains(bytes.ToLower(body), []byte("data-tm-products")) {
		return body
	}
	bar := []byte(productSwitcherHTML(currentPath))
	lower := bytes.ToLower(body)
	if i := bytes.Index(lower, []byte("</body>")); i >= 0 {
		out := make([]byte, 0, len(body)+len(bar))
		out = append(out, body[:i]...)
		out = append(out, bar...)
		out = append(out, body[i:]...)
		return out
	}
	return append(body, bar...)
}

func productSwitcherHTML(currentPath string) string {
	var links strings.Builder
	for _, btn := range productButtons {
		if !validProductPath(btn.Path) {
			continue
		}
		class := "tm-product"
		if currentPath == btn.Path || strings.HasPrefix(currentPath, btn.Path+"/") {
			class = "tm-product active"
		}
		links.WriteString(`<a class="`)
		links.WriteString(class)
		links.WriteString(`" href="`)
		links.WriteString(html.EscapeString(btn.Path))
		links.WriteString(`">`)
		links.WriteString(html.EscapeString(btn.Label))
		links.WriteString(`</a>`)
	}
	return `<nav data-tm-products aria-label="Products"><style data-tm-products>nav[data-tm-products]{position:fixed;left:12px;right:12px;bottom:12px;z-index:2147483646;display:flex;flex-wrap:wrap;gap:8px;justify-content:center;padding:10px;border-radius:16px;background:rgba(15,23,42,.92)}nav[data-tm-products] a{color:#fff;text-decoration:none;font:600 13px/1 system-ui,sans-serif;padding:8px 12px;border-radius:999px}nav[data-tm-products] a.active{background:#fff;color:#0f172a}</style>` + links.String() + `</nav>`
}

func validProductPath(path string) bool {
	return strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") && !strings.Contains(path, "://")
}
