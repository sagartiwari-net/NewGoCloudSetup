-- SeoGroupBuy reseller copies of official *.upays.in websites onto *.seogbtools.com.
-- Does not change upays.in rows. Skips other-friends recloud hosts (az/markh/amzc/tbaazar/shsm).
-- Live MySQL is STRICT; mysql CLI has no implicit defaults. Match panel inserts.

SET SESSION sql_mode = 'NO_ENGINE_SUBSTITUTION';

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, access_slug)
SELECT src.tool_id,
       CONCAT('SeoGroupBuy ', src.name),
       REPLACE(src.domain, '.upays.in', '.seogbtools.com'),
       CONCAT('secret_sgb_', src.id),
       30,
       src.default_limits_json,
       src.session_security_enabled,
       CONCAT(
         'sgb',
         IF(
           TRIM(COALESCE(src.access_slug, '')) = '',
           SUBSTRING_INDEX(src.domain, '.', 1),
           SUBSTRING_INDEX(src.access_slug, '-', 1)
         )
       )
  FROM websites src
 WHERE src.domain LIKE '%.upays.in'
   AND src.domain NOT REGEXP '^(az|markh|amzc|tbaazar|shsm)'
   AND NOT EXISTS (
         SELECT 1 FROM websites x
          WHERE x.domain = REPLACE(src.domain, '.upays.in', '.seogbtools.com')
       );

INSERT INTO accounts (website_id, name, cookie, user_agent_id, user_agent, proxy_id, proxy, status, show_limit, description, automation_task_uid, automation_ingest_key, cookie_updated_at, last_used_at, failure_count)
SELECT w.id, CONCAT(w.name, ' 1'), '', 0, '', 0, '', 'active', 0,
       'Paste this website''s own cookie. It is not shared with upays.in.',
       '', '', '', '', 0
  FROM websites w
 WHERE w.domain LIKE '%.seogbtools.com'
   AND NOT EXISTS (SELECT 1 FROM accounts a WHERE a.website_id = w.id);

INSERT INTO products (website_id, product_name, product_ids_json)
SELECT w.id, w.name, '["seogbtools"]'
  FROM websites w
 WHERE w.domain LIKE '%.seogbtools.com'
   AND NOT EXISTS (SELECT 1 FROM products p WHERE p.website_id = w.id);

INSERT IGNORE INTO operator_websites (operator_id, website_id)
SELECT o.id, w.id
  FROM operators o
  JOIN websites w ON w.domain LIKE '%.seogbtools.com'
 WHERE o.username = 'SeoGroupBuy';
