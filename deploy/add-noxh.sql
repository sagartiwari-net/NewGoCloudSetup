-- NoxTools Home (noxh.upays.in → port 6309) — temporary keepalive test
-- Run once on MySQL gt4rents. Does not modify other websites.

INSERT INTO tools (name, category, limits_json)
SELECT 'NoxTools Home', 'seo', '[{"key":"credits","label":"Credits","default":100}]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='NoxTools Home');

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, access_slug)
SELECT t.id, 'NoxTools Home', 'noxh.upays.in', 'secret_noxtools_home', 120, '{"credits":100}', 1, 'noxh'
FROM tools t
WHERE t.name='NoxTools Home'
  AND NOT EXISTS (
    SELECT 1 FROM websites
    WHERE name='NoxTools Home' OR domain IN ('noxh.upays.in','noxh.gt4rents.com')
  )
LIMIT 1;

INSERT INTO products (website_id, product_name, product_ids_json)
SELECT w.id, 'NoxTools Home', '["noxh"]'
FROM websites w
WHERE w.domain='noxh.upays.in'
  AND NOT EXISTS (SELECT 1 FROM products WHERE website_id=w.id AND product_name='NoxTools Home')
LIMIT 1;

-- Seed the overnight test cookie (PHPSESSID + amember_nr)
INSERT INTO accounts (website_id, name, cookie, user_agent, proxy, status, show_limit, description, cookie_updated_at, last_used_at)
SELECT w.id,
       'Nox Home Keepalive',
       '[{"domain":".noxtools.com","hostOnly":false,"httpOnly":true,"name":"amember_nr","path":"/","sameSite":null,"secure":true,"session":true,"storeId":null,"value":"febbc74bc2c9407a3228826abfa275e9"},{"domain":".noxtools.com","hostOnly":false,"httpOnly":true,"name":"PHPSESSID","path":"/","sameSite":null,"secure":true,"session":true,"storeId":null,"value":"er9j6jf4h586opeqjk2oon3pcm"}]',
       'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36',
       '',
       'active',
       0,
       'temporary overnight keepalive test',
       UTC_TIMESTAMP(),
       NULL
FROM websites w
WHERE w.domain='noxh.upays.in'
  AND NOT EXISTS (
    SELECT 1 FROM accounts a WHERE a.website_id=w.id AND a.name='Nox Home Keepalive'
  )
LIMIT 1;
