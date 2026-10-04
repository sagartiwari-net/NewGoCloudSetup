# Uncensored Chat Go Proxy (local)

Reverse proxy for **https://uncensored.chat** using DigitaVision / GoAuto **cookies**.

Required: **`uncensored_chat_session`** + **`XSRF-TOKEN`**. Prefer hostOnly `uncensored.chat`.

JSON array or GoAuto `{ cookies: [...] }` both work. **`referer` is optional.**

```bash
chmod +x start-local.sh
./start-local.sh
```

Open: **http://localhost:4891/**
