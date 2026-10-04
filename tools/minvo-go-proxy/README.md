# Minvo Go Proxy (local)

Reverse proxy for **https://app.minvo.pro** using a DigitaVision / GoAuto **localStorage** dump.

Required keys: **`auth_jwt`** and **`token`** (Cognito). `user_id` is useful. Array cookies are not required.

GoAuto wrap works with or without `referer`.

```bash
chmod +x start-local.sh
./start-local.sh
```

Open: **http://localhost:4871/84368/episodes**
