-- SeoGroupBuy extra Semrush (smrs2–5) + ChatGPT (cgpt2–3) on *.seogbtools.com.
-- Same Go ports as smrs/cgpt. Separate access_slug each. Product id 5. No accounts.
-- Does not change existing smrs.seogbtools.com / cgpt.seogbtools.com rows.

SET SESSION sql_mode = 'NO_ENGINE_SUBSTITUTION';

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, access_slug)
SELECT t.id,
       v.name,
       v.domain,
       CONCAT('secret_', REPLACE(v.domain, '.', '_')),
       COALESCE(src.session_duration, 30),
       COALESCE(NULLIF(TRIM(src.default_limits_json), ''), '{}'),
       1,
       v.slug
  FROM (
        SELECT 'Semrush' tool_name, 'SeoGroupBuy Semrush 2' name, 'smrs2.seogbtools.com' domain, 'smrs.seogbtools.com' owner, 'sgbsmrs2' slug
        UNION ALL SELECT 'Semrush', 'SeoGroupBuy Semrush 3', 'smrs3.seogbtools.com', 'smrs.seogbtools.com', 'sgbsmrs3'
        UNION ALL SELECT 'Semrush', 'SeoGroupBuy Semrush 4', 'smrs4.seogbtools.com', 'smrs.seogbtools.com', 'sgbsmrs4'
        UNION ALL SELECT 'Semrush', 'SeoGroupBuy Semrush 5', 'smrs5.seogbtools.com', 'smrs.seogbtools.com', 'sgbsmrs5'
        UNION ALL SELECT 'ChatGPT', 'SeoGroupBuy ChatGPT 2', 'cgpt2.seogbtools.com', 'cgpt.seogbtools.com', 'sgbcgpt2'
        UNION ALL SELECT 'ChatGPT', 'SeoGroupBuy ChatGPT 3', 'cgpt3.seogbtools.com', 'cgpt.seogbtools.com', 'sgbcgpt3'
       ) v
  JOIN tools t ON t.name = v.tool_name
  LEFT JOIN websites src ON src.domain = v.owner
 WHERE NOT EXISTS (SELECT 1 FROM websites x WHERE x.domain = v.domain)
   AND NOT EXISTS (SELECT 1 FROM websites x WHERE x.access_slug = v.slug);

INSERT INTO products (website_id, product_name, product_ids_json)
SELECT w.id, w.name, '["5"]'
  FROM websites w
 WHERE w.domain IN (
       'smrs2.seogbtools.com',
       'smrs3.seogbtools.com',
       'smrs4.seogbtools.com',
       'smrs5.seogbtools.com',
       'cgpt2.seogbtools.com',
       'cgpt3.seogbtools.com'
     )
   AND NOT EXISTS (SELECT 1 FROM products p WHERE p.website_id = w.id);

INSERT IGNORE INTO operator_websites (operator_id, website_id)
SELECT o.id, w.id
  FROM operators o
  JOIN websites w ON w.domain IN (
       'smrs2.seogbtools.com',
       'smrs3.seogbtools.com',
       'smrs4.seogbtools.com',
       'smrs5.seogbtools.com',
       'cgpt2.seogbtools.com',
       'cgpt3.seogbtools.com'
     )
 WHERE o.username = 'SeoGroupBuy';

INSERT INTO telegram_routes (website_id, events_json)
SELECT w.id, '["logout","spam"]'
  FROM websites w
 WHERE w.domain IN (
       'smrs2.seogbtools.com',
       'smrs3.seogbtools.com',
       'smrs4.seogbtools.com',
       'smrs5.seogbtools.com',
       'cgpt2.seogbtools.com',
       'cgpt3.seogbtools.com'
     )
   AND NOT EXISTS (SELECT 1 FROM telegram_routes r WHERE r.website_id = w.id);

INSERT IGNORE INTO telegram_route_chats (route_id, destination_id)
SELECT r.id, d.id
  FROM telegram_routes r
  JOIN websites w ON w.id = r.website_id
  JOIN operators o ON o.username = 'SeoGroupBuy'
  JOIN telegram_destinations d ON d.reseller_id = o.id AND d.enabled = 1
 WHERE w.domain IN (
       'smrs2.seogbtools.com',
       'smrs3.seogbtools.com',
       'smrs4.seogbtools.com',
       'smrs5.seogbtools.com',
       'cgpt2.seogbtools.com',
       'cgpt3.seogbtools.com'
     );
