const { createServer } = require("node:http")
const { parse } = require("node:url")
const fs = require("node:fs")
const path = require("node:path")
const next = require("next")

const port = Number(process.env.PORT || 3000)
const hostname = "127.0.0.1"
const app = next({ dev: false, hostname, port })
const handle = app.getRequestHandler()

// A refresh can still be showing the previous build's HTML. Those script
// URLs 404, so the page stays on "Redirecting to sign in". Answer the
// missing file with a reload onto ?__v=, which is not the cached document.
// Already on ?__v= means this build's own file is missing: do not loop.
const RELOAD =
  '(function(){try{var u=new URL(location.href);if(u.searchParams.has("__v"))return;u.searchParams.set("__v",String(Date.now()));location.replace(u.toString())}catch(e){}})();'

function staticFileExists(pathname) {
  let rel = pathname.slice("/_next/static/".length)
  try {
    rel = decodeURIComponent(rel)
  } catch {
    return false
  }
  if (!rel || rel.includes("\0") || rel.split("/").includes("..")) return false
  return fs.existsSync(path.join(process.cwd(), ".next", "static", rel))
}

app.prepare().then(() => {
  createServer((req, res) => {
    // Chrome sends "gzip, deflate, br, zstd". Next has a year-long prerender
    // (x-nextjs-cache: HIT, s-maxage=31536000) stored for that exact header,
    // and that copy still says "Redirecting to sign in".
    const encoding = String(req.headers["accept-encoding"] || "")
    if (encoding.includes("zstd") || encoding.includes("br")) {
      req.headers["accept-encoding"] = "gzip"
    }
    const parsed = parse(req.url || "/", true)
    const pathname = parsed.pathname || "/"
    if (pathname.startsWith("/_next/static/") && !staticFileExists(pathname)) {
      res.statusCode = 200
      res.setHeader("Content-Type", "application/javascript; charset=utf-8")
      res.setHeader("Cache-Control", "no-store")
      res.setHeader("Access-Control-Allow-Origin", "*")
      res.setHeader("X-Content-Type-Options", "nosniff")
      res.end(RELOAD)
      return
    }
    handle(req, res, parsed).catch((err) => {
      console.error(err && err.message ? err.message : err)
      if (!res.headersSent) {
        res.statusCode = 500
        res.end("panel ui error")
      }
    })
  }).listen(port, hostname, () => {
    console.log(`panel ui on http://${hostname}:${port}`)
  })
})
