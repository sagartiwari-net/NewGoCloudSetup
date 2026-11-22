-- T4 Toolz ChatGPT + Claude AI (wildcard *.t4toolz.com already on box).
-- cgpt.t4toolz.com → 5151 (slug t4cgpt)
-- clud.t4toolz.com → 5171 (slug t4clud)
-- Product placeholder ["t4toolz"]. No accounts — map cookies in panel.
-- Assigns same reseller(s) that already own ah2.t4toolz.com.

SET SESSION sql_mode = 'NO_ENGINE_SUBSTITUTION';

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, access_slug)
SELECT t.id,
       'T4 Toolz ChatGPT',
       'cgpt.t4toolz.com',
       'secret_cgpt_t4toolz_com',
       COALESCE(src.session_duration, 30),
       COALESCE(NULLIF(TRIM(src.default_limits_json), ''), '{}'),
       1,
       't4cgpt'
  FROM tools t
  LEFT JOIN websites src ON src.domain = 'cgpt.upays.in'
 WHERE t.name = 'ChatGPT'
   AND NOT EXISTS (SELECT 1 FROM websites x WHERE x.domain = 'cgpt.t4toolz.com')
   AND NOT EXISTS (SELECT 1 FROM websites x WHERE x.access_slug = 't4cgpt');

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, access_slug)
SELECT t.id,
       'T4 Toolz Claude AI',
       'clud.t4toolz.com',
       'secret_clud_t4toolz_com',
       COALESCE(src.session_duration, 30),
       COALESCE(NULLIF(TRIM(src.default_limits_json), ''), '{}'),
       1,
       't4clud'
  FROM tools t
  LEFT JOIN websites src ON src.domain = 'clud.upays.in'
 WHERE t.name = 'Claude AI'
   AND NOT EXISTS (SELECT 1 FROM websites x WHERE x.domain = 'clud.t4toolz.com')
   AND NOT EXISTS (SELECT 1 FROM websites x WHERE x.access_slug = 't4clud');

UPDATE websites SET access_slug = 't4cgpt', name = 'T4 Toolz ChatGPT'
 WHERE domain = 'cgpt.t4toolz.com'
   AND (access_slug IS NULL OR TRIM(access_slug) = '')
   AND NOT EXISTS (SELECT 1 FROM websites x WHERE x.access_slug = 't4cgpt' AND x.domain <> 'cgpt.t4toolz.com');

UPDATE websites SET access_slug = 't4clud', name = 'T4 Toolz Claude AI'
 WHERE domain = 'clud.t4toolz.com'
   AND (access_slug IS NULL OR TRIM(access_slug) = '')
   AND NOT EXISTS (SELECT 1 FROM websites x WHERE x.access_slug = 't4clud' AND x.domain <> 'clud.t4toolz.com');

INSERT INTO products (website_id, product_name, product_ids_json)
SELECT w.id, w.name, '["t4toolz"]'
  FROM websites w
 WHERE w.domain IN ('cgpt.t4toolz.com', 'clud.t4toolz.com')
   AND NOT EXISTS (SELECT 1 FROM products p WHERE p.website_id = w.id);

INSERT IGNORE INTO operator_websites (operator_id, website_id)
SELECT ow.operator_id, w.id
  FROM websites w
  JOIN operator_websites ow ON ow.website_id = (SELECT id FROM websites WHERE domain = 'ah2.t4toolz.com' LIMIT 1)
 WHERE w.domain IN ('cgpt.t4toolz.com', 'clud.t4toolz.com');

INSERT INTO telegram_routes (website_id, events_json)
SELECT w.id, '["logout","spam"]'
  FROM websites w
 WHERE w.domain IN ('cgpt.t4toolz.com', 'clud.t4toolz.com')
   AND NOT EXISTS (SELECT 1 FROM telegram_routes r WHERE r.website_id = w.id);

INSERT IGNORE INTO telegram_route_chats (route_id, destination_id)
SELECT r.id, d.id
  FROM telegram_routes r
  JOIN websites w ON w.id = r.website_id AND w.domain IN ('cgpt.t4toolz.com', 'clud.t4toolz.com')
  JOIN operator_websites ow ON ow.website_id = w.id
  JOIN telegram_destinations d ON d.reseller_id = ow.operator_id AND d.enabled = 1;
