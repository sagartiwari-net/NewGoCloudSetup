# Reseller websites

Ek tool ka ek hi process aur ek hi port rehta hai. Dost ka domain usi port par aata hai, lekin panel mein uski website alag hoti hai. Cookie, limit, session, logs, aur Telegram usi website par rehte hain. gt4rents wali website ke saath mix nahi hote.

Server IP wahi hai jo `gt4rents.com` use karta hai: `65.109.16.196`.

## Naye dost ko kaise dena hai

Code, map file, aur `gt4rents.com` ka nginx mat chhedo. Har tool ka process pehle se chal raha hota hai. Naya dost = DNS + aaPanel ki alag site + panel ki alag website.

1. Panel mein reseller banao. Username yaad rakhna. Role Reseller. Status active. Telegram chat usi reseller par add karo.
2. Cloudflare par A record, DNS only, content `65.109.16.196`. Name wahi rakho jo URL mein chahiye, jaise `refs`.
3. aaPanel → Website → Add site. Domain sirf woh ek host, jaise `refs.dost.com`. PHP **Static**. FTP aur database mat banao.
4. Us site ki Nginx config mein `location /` ko neeche wale reverse proxy se badlo. Port us tool ka purana port hai. Naya process mat chalao.
5. Us site par SSL → Let's Encrypt. DNS only isiliye sahi hai, certificate isi site ka banega.
6. Panel → Websites → Add. Tool wahi choose karo, domain bilkul us host ke barabar. Reseller ko sirf ye website assign karo. Cookie isi account mein paste karo. Product id asli aMember id lagao. Telegram route isi website par us reseller ke chat se jodo.
7. Reseller login se check karo: sirf uski websites, uska cookie, uske logs.

Example, Ahrefs hamesha port `5291`:

```nginx
location / {
    proxy_pass http://127.0.0.1:5291;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $connection_upgrade;
    client_max_body_size 100m;
    proxy_read_timeout 3600s;
}
```

`connection_upgrade` wahi map hai jo `gt4rents.com` wali config ke http block mein pehle se hai. Semrush ke liye port `5141`, Canva `4501`, Grammarly `4911`. Koi aur tool ho to `tools.json` ka port use karo.

Jo cheez share hoti hai: binary, port, process, static cache.

Jo cheez alag rehti hai: domain, cookie, limit, users, sessions, logs, Telegram.

## ToolWaly

Reseller username: `ToolWaly`  
Domain: `toolwaly.com`

| Tool | URL | Same port | gt4rents site (share nahi) |
|------|-----|-----------|----------------------------|
| Ahrefs | `https://refs.toolwaly.com` | 5291 | `refs.gt4rents.com` |
| Semrush | `https://smrs.toolwaly.com` | 5141 | `smrs.gt4rents.com` |
| Canva | `https://cnva.toolwaly.com` | 4501 | `cnva.gt4rents.com` |
| Grammarly | `https://grammarly.toolwaly.com` | 4911 | `grammarly.gt4rents.com` |

Panel website names: ToolWaly Ahrefs, ToolWaly Semrush, ToolWaly Canva, ToolWaly Grammarly.  
Har ek ka apna account hai. Cookie abhi khali hai. Product id placeholder `toolwaly` hai, access link se pehle asli aMember product id daalni hai.

Telegram sirf ToolWaly ke destinations par jaata hai, aur sirf in char websites ke logout/spam events par. Destinations panel mein pehle se hon to panel API restart unhe route se jod deta hai. Abhi koi chat nahi hai to pehle chat add karo, phir panel API restart.

Agar ToolWaly reseller par gt4rents wali Semrush, Envato, ya ChatGPT website bhi assigned hai, use hata dena. Warna us login par woh shared rows bhi dikhengi.

## Cloudflare (toolwaly.com)

Ye char A records `65.109.16.196` par hain, DNS only: `refs`, `smrs`, `cnva`, `grammarly`. Grey cloud rehne do. SSL aaPanel ki alag site par Let's Encrypt se lagega.

## aaPanel

`gt4rents.com` ki config mat badalna. Char alag sites banao.

| Add site domain | Reverse proxy |
|-----------------|---------------|
| `refs.toolwaly.com` | `http://127.0.0.1:5291` |
| `smrs.toolwaly.com` | `http://127.0.0.1:5141` |
| `cnva.toolwaly.com` | `http://127.0.0.1:4501` |
| `grammarly.toolwaly.com` | `http://127.0.0.1:4911` |

Har site: PHP Static, phir upar wala `location /` block, phir SSL → Let's Encrypt.

Panel rows ke liye ek baar:

```bash
cd /www/wwwroot/gt4rents.com/_repo && git pull
./deploy/build-panel.sh && ./deploy/start-panel-api.sh
```

Panel log mein ye lines aani chahiye:

```text
added ToolWaly Ahrefs website … at refs.toolwaly.com
added ToolWaly Semrush website … at smrs.toolwaly.com
added ToolWaly Canva website … at cnva.toolwaly.com
added ToolWaly Grammarly website … at grammarly.toolwaly.com
```

```bash
tail -n 80 -f \
  /www/wwwroot/gt4rents.com/_repo/panel/update-panel/panel-ui.log \
  /www/wwwroot/gt4rents.com/panel/panel-api.log \
  /www/wwwlogs/panel.gt4rents.com.log \
  /www/wwwlogs/panel.gt4rents.com.error.log
```

## Baaki resellers

Inke alag domain abhi nahi hain. Panel seed unhe gt4rents wali website rows se jodta hai, isliye cookie aur logs abhi share hain. Alag chahiye to upar wala “naye dost” flow use karo.

| Reseller | Abhi linked tools | URL abhi |
|----------|-------------------|----------|
| SemrushToolz | Semrush, ChatGPT, Envato, Ubersuggest, Helium 10 | `smrs.gt4rents.com`, `cgpt.gt4rents.com`, `envt.gt4rents.com`, `ubersuggest.gt4rents.com`, `helium10.gt4rents.com` |
| SeoGroupBuy | Semrush | `smrs.gt4rents.com` |
| AmzPremiumToolz | Helium 10, ChatGPT, Claude AI | `helium10.gt4rents.com`, `cgpt.gt4rents.com`, `clud.gt4rents.com` |

Seed tool name se pehli website uthata hai. Alag domain tab tak nahi hai jab tak upar wala flow na chalaya jaye.
