-- Tanveer / PrideSeoTools Ahrefs on refs.prideseotools.com (port 5291).
-- Access slug: pdrefs (pd + refs). Product placeholder ["prideseotools"]. No accounts.
-- Reseller username must be: Tanveer (panel seed creates it on API start if missing).

SET SESSION sql_mode = 'NO_ENGINE_SUBSTITUTION';

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, access_slug)
SELECT t.id,
       'Tanveer Ahrefs',
       'refs.prideseotools.com',
       'secret_refs_prideseotools_com',
       COALESCE(src.session_duration, 30),
       COALESCE(NULLIF(TRIM(src.default_limits_json), ''), '{}'),
       1,
       'pdrefs'
  FROM tools t
  LEFT JOIN websites src ON src.domain = 'refs.upays.in'
 WHERE t.name = 'Ahrefs'
   AND NOT EXISTS (SELECT 1 FROM websites x WHERE x.domain = 'refs.prideseotools.com')
   AND NOT EXISTS (SELECT 1 FROM websites x WHERE x.access_slug = 'pdrefs');

UPDATE websites
   SET access_slug = 'pdrefs',
       name = 'Tanveer Ahrefs'
 WHERE domain = 'refs.prideseotools.com'
   AND (access_slug IS NULL OR TRIM(access_slug) = '')
   AND NOT EXISTS (SELECT 1 FROM websites x WHERE x.access_slug = 'pdrefs' AND x.domain <> 'refs.prideseotools.com');

INSERT INTO products (website_id, product_name, product_ids_json)
SELECT w.id, w.name, '["prideseotools"]'
  FROM websites w
 WHERE w.domain = 'refs.prideseotools.com'
   AND NOT EXISTS (SELECT 1 FROM products p WHERE p.website_id = w.id);

INSERT IGNORE INTO operator_websites (operator_id, website_id)
SELECT o.id, w.id
  FROM operators o
  JOIN websites w ON w.domain = 'refs.prideseotools.com'
 WHERE o.username = 'Tanveer';

INSERT INTO telegram_routes (website_id, events_json)
SELECT w.id, '["logout","spam"]'
  FROM websites w
 WHERE w.domain = 'refs.prideseotools.com'
   AND NOT EXISTS (SELECT 1 FROM telegram_routes r WHERE r.website_id = w.id);

INSERT IGNORE INTO telegram_route_chats (route_id, destination_id)
SELECT r.id, d.id
  FROM telegram_routes r
  JOIN websites w ON w.id = r.website_id AND w.domain = 'refs.prideseotools.com'
  JOIN operators o ON o.username = 'Tanveer'
  JOIN telegram_destinations d ON d.reseller_id = o.id AND d.enabled = 1;
