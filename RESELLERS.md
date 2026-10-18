# Reseller websites

Ek tool ka ek hi process aur ek hi port rehta hai. Dost ka domain usi port par aata hai, lekin panel mein uski website alag hoti hai. Cookie, limit, session, logs, aur Telegram usi website par rehte hain. gt4rents wali website ke saath mix nahi hote.

Server IP wahi hai jo `gt4rents.com` use karta hai: `65.109.16.196`.

## Naye dost ko kaise dena hai

1. Panel mein reseller banao. Username yaad rakhna. Role Reseller. Status active.
2. Uske Telegram chats Panel → Telegram destinations mein usi reseller ke naam par add karo. Chat id group ki ho, bot us group mein ho.
3. Neeche wale domain pattern se uske hosts choose karo. Example: `ahrefs.dost.com`.
4. `deploy/nginx-host-port.map.conf` mein har host ko us tool ke **existing** port par map karo. Naya port mat banana.
5. `deploy/nginx-wildcard.server.conf` ke `server_name` mein wahi hosts add karo. Live aaPanel wale 80 aur 443 server block mein bhi add karo. Naya site mat banao.
6. Panel API mein us dost ke liye `ensure...Sites` jaisa seed add karo, jo:
   - tool ka same `tool_id` use kare
   - naya `websites.domain` insert kare
   - limits owner website se copy kare, taaki baad mein sirf uski website par number change ho
   - khali account banaye, cookie baad mein paste ho
   - `operator_websites` mein sirf us reseller ko link kare
   - us reseller ke telegram destinations ko us website ki route par lagaye
7. Cloudflare par us domain ke A records server IP par, orange cloud.
8. Server par `git pull`, nginx map copy, `nginx -t && nginx -s reload`, phir `./deploy/build-panel.sh && ./deploy/start-panel-api.sh`.
9. Jin tools ke proxy code mein host change hua ho, unhe `./deploy/build-one.sh <sub> && ./deploy/start-tool.sh <sub>` se rebuild karo.
10. Panel mein us website ka product id asli aMember id se badlo, aur account cookie paste karo.
11. Reseller login se check karo: sirf uski websites dikhein, dusre dost ya gt4rents ke cookies aur logs nahi.

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

Ye char A records `65.109.16.196` par hain: `refs`, `smrs`, `cnva`, `grammarly`.

Abhi Proxy status **DNS only** hai. Origin certificate sirf `*.gt4rents.com` ka hai, isliye grey cloud par `https://` certificate warning dikhayega. Har record ke cloud icon ko **Proxied** (orange) karo. SSL/TLS mode **Full** rakho. Flexible mat use karna.

## aaPanel / nginx

Naya website create mat karna. Jo site `*.gt4rents.com` ko tool port par bhejti hai, usi ke `server_name` mein ye char hosts add karne hain. Port 80 aur port 443 dono blocks mein.

```nginx
server_name *.gt4rents.com gt4rents.com refs.toolwaly.com smrs.toolwaly.com cnva.toolwaly.com grammarly.toolwaly.com;
```

Phir map update:

```bash
cd /www/wwwroot/gt4rents.com/_repo && git pull
cp deploy/nginx-host-port.map.conf /www/server/panel/vhost/nginx/gt4rents-host-port.map.conf
nginx -t && nginx -s reload
./deploy/build-one.sh refs && ./deploy/start-tool.sh refs
./deploy/build-one.sh smrs && ./deploy/start-tool.sh smrs
./deploy/build-one.sh cnva && ./deploy/start-tool.sh cnva
./deploy/build-one.sh grammarly && ./deploy/start-tool.sh grammarly
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
