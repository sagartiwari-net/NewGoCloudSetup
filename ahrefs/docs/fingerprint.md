# Session fingerprint bind

## Goal

A copied `tb.*` session token cannot be used from another browser profile / modified extension without the matching device fingerprint.

## How

1. Extension (`tm-fp.js`) creates one random id `tmfp.<uuid>` in `chrome.storage.local` (`tm_bar_fp`).
2. On mint (`/v1/session/verify` with aMember cookie), server stores fingerprint on the session row.
3. Every later call (`verify` revalidate, `/v1/me`, `/v4/*`) must send the same value via `X-TM-Bar-FP` (and/or JSON `fingerprint`).
4. DNR also injects `X-TM-Bar-FP` on proxied v4 requests.

## Config

| Env | Default | Meaning |
|-----|---------|---------|
| `AHREFS_BAR_REQUIRE_FP` | `1` | Require + enforce bind |
| set to `0` | — | Soft mode (legacy / debug only) |

## Attack notes

- Token alone → rejected (`fingerprint_required` / `fingerprint_mismatch`).
- Full storage dump (token + `tm_bar_fp`) still works from that profile — FP is anti-token-exfil, not DRM.
- Clearing extension storage creates a new FP → old sessions fail until re-login.
