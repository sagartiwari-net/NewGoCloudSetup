# Static cache update — working tools only

Date: 2026-10-12. Source list: `ROLLOUT.md` section **Fixed** (verified on an access link).

What changed: static JS, CSS, fonts, and images that these proxies already cache now tell the browser

`Cache-Control: public, max-age=31536000, immutable`

HTML, JSON, and APIs stay `no-store`. A new release URL is a new file, so the browser fetches it once and keeps it for a year. Reload no longer re-downloads the old file.

`refs` (Ahrefs) already had this, plus brotli/gzip stored on disk and ETag. The other tools keep their existing disk or memory cache format. Only the browser header was raised to the same 1-year rule. Their cache code is not the same file, so the Ahrefs disk format was not copied into them.

Not started and “still an issue” tools were left alone.

## Done

| Sub | Folder | Note |
|-----|--------|------|
| `refs` | `ahrefs-go-proxy` | Already done earlier (1-year immutable, brotli + gzip on disk, ETag). Not edited in this pass. |
| `answerthepublic` | `answerthepublic-go-proxy` | Static header raised to 1 year. |
| `cgpt` | `chatgpt-go-proxy` | Extra CDN fallback raised to 1 year. Upstream cache header is still kept when Ahrefs/ChatGPT sends one. |
| `cnva` | `canva-go-proxy` | Static header raised to 1 year. |
| `copyspace` | `copyspace-go-proxy` | Static header raised to 1 year. |
| `coursera` | `coursera-go-proxy` | Static header raised to 1 year. |
| `cramly` | `cramly-go-proxy` | Static header raised to 1 year. |
| `digen` | `digen-go-proxy` | Static header raised to 1 year. |
| `educative` | `educative-go-proxy` | Static header raised to 1 year. |
| `envt` | `envato-go-proxy` | Static header raised to 1 year. |
| `epidemicsound` | `epidemicsound-go-proxy` | Static header raised to 1 year. |
| `erank` | `erank-go-proxy` | Was 7 days. Now 1 year. |
| `flexclip` | `flexclip-go-proxy` | Static header raised to 1 year. |
| `fishaudio` | `fishaudio-go-proxy` | Static header raised to 1 year. |
| `glorify` | `glorify-go-proxy` | Static header raised to 1 year. |
| `gptzero` | `gptzero-go-proxy` | Was 1 day + immutable. Now 1 year. |
| `grammarly` | `grammarly-go-proxy` | Was 7 days. Now 1 year. |
| `helium10` | `helium10-go-proxy` | Static header raised to 1 year. |
| `ilovepdf` | `ilovepdf-go-proxy` | Static header raised to 1 year. |
| `imgupscaler` | `imgupscaler-go-proxy` | Static header raised to 1 year. |
| `jasper` | `jasper-go-proxy` | Static header raised to 1 year. |
| `joggai` | `joggai-go-proxy` | Static header raised to 1 year. |
| `junglescout` | `junglescout-go-proxy` | Static header raised to 1 year. |
| `leonardo` | `leonardo-go-proxy` | Was 7 days. Now 1 year. |
| `linkedinlearning` | `linkedin-learning-go-proxy` | Static header raised to 1 year. |
| `mgfc` | `magnific-go-proxy` | Was 7 days. Now 1 year. |
| `minvo` | `minvo-go-proxy` | Static header raised to 1 year. |
| `mojo` | `mojo-go-proxy` | Static header raised to 1 year. |
| `piktochart` | `piktochart-go-proxy` | Static header raised to 1 year. |
| `pixlr` | `pixlr-go-proxy` | Static header raised to 1 year. |
| `placeit` | `placeit-go-proxy` | Static header raised to 1 year. |
| `ppspy` | `ppspy-go-proxy` | Static header raised to 1 year. |
| `prezi` | `prezi-go-proxy` | Static header raised to 1 year. |
| `quillbot` | `quillbot-go-proxy` | Static header raised to 1 year. |
| `rivalflow` | `rivalflow-go-proxy` | Static header raised to 1 year. |
| `screpy` | `screpy-go-proxy` | Static header raised to 1 year. |
| `scribd` | `scribd-go-proxy` | Static header raised to 1 year. |
| `searchatlas` | `searchatlas-go-proxy` | Static header raised to 1 year. |
| `sellthetrend` | `sellthetrend-go-proxy` | Static header raised to 1 year. |
| `seobility` | `seobility-go-proxy` | Static header raised to 1 year. |
| `seositecheckup` | `seositecheckup-go-proxy` | Static header raised to 1 year. |
| `seotesteronline` | `seotesteronline-go-proxy` | Static header raised to 1 year. |
| `similarweb` | `similarweb-go-proxy` | Static header raised to 1 year. |
| `sketchgenius` | `sketchgenius-go-proxy` | Static header raised to 1 year. |
| `slidebean` | `slidebean-go-proxy` | Static header raised to 1 year. |
| `smrs` | `semrush-new-go-proxy` | Static CDN header raised to 1 year. |
| `spyfu` | `spyfu-go-proxy` | Static header raised to 1 year. |
| `storybase` | `storybase-go-proxy` | Static header raised to 1 year. |
| `syntx` | `syntx-go-proxy` | Static header raised to 1 year. |
| `videotoblog` | `videotoblog-go-proxy` | Static header raised to 1 year. |
| `wordtune` | `wordtune-go-proxy` | Static header raised to 1 year. |
| `writecream` | `writecream-go-proxy` | Static header raised to 1 year. |
| `writehuman` | `writehuman-go-proxy` | New official tool. Hashed JS, CSS, fonts, and images use disk `cdn-cache/` (72h) and `Cache-Control: public, max-age=31536000, immutable`. HTML and API stay `no-store`. |
| `wrank` | `woorank-go-proxy` | Static header raised to 1 year. |

## Not done

These Fixed tools do not have a static JS/CSS cache header to extend. A new cache layer was not added, so a logged-in page or API response cannot be stored by mistake.

| Sub | Folder | Why |
|-----|--------|-----|
| `airbrush` | `airbrush-go-proxy` | No static `Cache-Control` in the proxy. |
| `clud` | `claude-ai-go-proxy` | No static `Cache-Control` in the proxy. |
| `closerscopy` | `closerscopy-go-proxy` | No static `Cache-Control` in the proxy. |
| `selleramp` | `selleramp-go-proxy` | No static `Cache-Control` in the proxy. |
| `shortform` | `shortform-go-proxy` | No static `Cache-Control` in the proxy. |

## Not touched

`ROLLOUT.md` **Not started** and **Tried — still an issue**. Those proxies were not edited.
