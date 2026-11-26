# RankyTools — URL inventory

Local ports use the **6500** recloud block (seoshop owns `6400–6499`). Server hosts are `*.upays.in`.

| # | Tool | Member URL | Local slug | Port | Local OK | Security | Server |
|---|------|------------|------------|------|----------|----------|--------|
| 1 | Semrush | https://sr.rankytools.com/ | ryksm1 | 6500 | local | panel + device | `ryksm1.upays.in` |
| 2 | Majestic | https://mj.rankytools.com/ | rykmj | 6501 | local | panel + device | `rykmj.upays.in` |
| 3 | ChatGPT | https://cp.rankytools.com/ | rykcp | 6502 | local | panel + device | `rykcp.upays.in` |
| 4 | Ubersuggest | https://us.rankytools.com/ | rykus | 6503 | local | panel + device | `rykus.upays.in` |
| 5 | Grammarly | https://gm.rankytools.com/ | rykgm | 6504 | local | panel + device | `rykgm.upays.in` |
| 6 | Envato Elements | https://en.rankytools.com/ | ryken | 6505 | local | panel + device | `ryken.upays.in` |
| 7 | Storyblocks | https://sb.rankytools.com/ | ryksb | 6506 | local | panel + device | `ryksb.upays.in` |
| 8 | SpyFu | https://sf.rankytools.com/ | ryksf | 6507 | local | panel + device | `ryksf.upays.in` |
| 9 | Niche Scraper | https://ns.rankytools.com/ | rykns | 6508 | local | panel + device | `rykns.upays.in` |
| 10 | SpamZilla | https://sz02.rankytools.com/ | ryksz02 | 6509 | local | panel + device | `ryksz02.upays.in` |
| 11 | SimilarWeb | https://sw.rankytools.com/ | ryksw | 6510 | local | panel + device | `ryksw.upays.in` |
| — | Keyword Everywhere | https://ke.rankytools.com/ | ~~rykke~~ | — | deleted (reCAPTCHA domain-bound; revisit later) | — | — |

## Server deploy

```bash
cd /www/wwwroot/gt4rents.com/_repo
git pull --ff-only origin main
chmod +x deploy/push-rankytools-ryk-tools.sh
./deploy/push-rankytools-ryk-tools.sh
```

Panel → **Recloud**: paste/update `PHPSESSID` cookie per row. Open only via panel access link.
