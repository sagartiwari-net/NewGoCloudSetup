-- ToolWaly Envato on envt.toolwaly.com (port 5261). Product placeholder ["toolwaly"]. No accounts.

SET SESSION sql_mode = 'NO_ENGINE_SUBSTITUTION';

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, access_slug)
SELECT t.id,
       'ToolWaly Envato',
       'envt.toolwaly.com',
       'secret_envt_toolwaly_com',
       COALESCE(src.session_duration, 30),
       COALESCE(NULLIF(TRIM(src.default_limits_json), ''), '{}'),
       1,
       'twenvt'
  FROM tools t
  LEFT JOIN websites src ON src.domain = 'envt.upays.in'
 WHERE t.name = 'Envato'
   AND NOT EXISTS (SELECT 1 FROM websites x WHERE x.domain = 'envt.toolwaly.com')
   AND NOT EXISTS (SELECT 1 FROM websites x WHERE x.access_slug = 'twenvt');

UPDATE websites
   SET access_slug = 'twenvt'
 WHERE domain = 'envt.toolwaly.com'
   AND (access_slug IS NULL OR TRIM(access_slug) = '')
   AND NOT EXISTS (SELECT 1 FROM websites x WHERE x.access_slug = 'twenvt' AND x.domain <> 'envt.toolwaly.com');

INSERT INTO products (website_id, product_name, product_ids_json)
SELECT w.id, w.name, '["toolwaly"]'
  FROM websites w
 WHERE w.domain = 'envt.toolwaly.com'
   AND NOT EXISTS (SELECT 1 FROM products p WHERE p.website_id = w.id);

INSERT IGNORE INTO operator_websites (operator_id, website_id)
SELECT o.id, w.id
  FROM operators o
  JOIN websites w ON w.domain = 'envt.toolwaly.com'
 WHERE o.username = 'ToolWaly';

INSERT INTO telegram_routes (website_id, events_json)
SELECT w.id, '["logout","spam"]'
  FROM websites w
 WHERE w.domain = 'envt.toolwaly.com'
   AND NOT EXISTS (SELECT 1 FROM telegram_routes r WHERE r.website_id = w.id);

INSERT IGNORE INTO telegram_route_chats (route_id, destination_id)
SELECT r.id, d.id
  FROM telegram_routes r
  JOIN websites w ON w.id = r.website_id AND w.domain = 'envt.toolwaly.com'
  JOIN operators o ON o.username = 'ToolWaly'
  JOIN telegram_destinations d ON d.reseller_id = o.id AND d.enabled = 1;
