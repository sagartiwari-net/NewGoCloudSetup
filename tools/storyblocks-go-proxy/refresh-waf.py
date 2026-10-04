#!/usr/bin/env python3
"""Open the real Storyblocks site, solve AWS WAF, and write aws-waf-token back to cookie.txt."""
import json
import sys
import time
from pathlib import Path

from selenium import webdriver
from selenium.webdriver.chrome.options import Options

root = Path(__file__).resolve().parent
cookie_path = root / "cookie.txt"
cookies = json.loads(cookie_path.read_text())

opts = Options()
opts.add_argument("--headless=new")
opts.add_argument("--disable-gpu")
opts.add_argument("--window-size=1440,900")
opts.add_argument("--disable-blink-features=AutomationControlled")
opts.add_experimental_option("excludeSwitches", ["enable-automation"])
opts.add_experimental_option("useAutomationExtension", False)

driver = webdriver.Chrome(options=opts)
driver.execute_cdp_cmd(
    "Page.addScriptToEvaluateOnNewDocument",
    {"source": "Object.defineProperty(navigator,'webdriver',{get:()=>undefined})"},
)
try:
    driver.get("https://www.storyblocks.com/")
    for c in cookies:
        ck = {"name": c["name"], "value": c["value"], "path": c.get("path") or "/"}
        if c.get("domain"):
            ck["domain"] = c["domain"]
        if c.get("secure"):
            ck["secure"] = True
        if c.get("httpOnly"):
            ck["httpOnly"] = True
        try:
            driver.add_cookie(ck)
        except Exception:
            pass
    driver.get("https://www.storyblocks.com/")
    token = ""
    deadline = time.time() + 40
    while time.time() < deadline:
        src = driver.page_source or ""
        waf = driver.get_cookie("aws-waf-token") or {}
        token = waf.get("value") or ""
        if token and "AwsWafIntegration" not in src and "My Account" in src:
            break
        time.sleep(2)
    if not token or "AwsWafIntegration" in (driver.page_source or ""):
        print("waf refresh failed", file=sys.stderr)
        sys.exit(1)
    for c in cookies:
        if c.get("name") == "aws-waf-token":
            c["value"] = token
            break
    else:
        cookies.append({
            "name": "aws-waf-token",
            "value": token,
            "domain": ".www.storyblocks.com",
            "path": "/",
            "secure": True,
            "httpOnly": False,
            "hostOnly": False,
        })
    cookie_path.write_text(json.dumps(cookies, indent=2) + "\n")
    print("waf refreshed", len(token))
finally:
    driver.quit()
