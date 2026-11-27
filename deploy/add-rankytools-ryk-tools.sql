-- RankyTools (ryksm1…ryksw) as panel recloud websites.
-- Each site has its own accounts row so cookies update separately in Recloud UI.
-- session_security_enabled=1 → panel access link + device lock.
-- Run on MySQL gt4rents after code deploy (or rely on ensureRecloudSite on first start).

INSERT INTO tools (name, category, limits_json)
SELECT 'RankyTools Semrush', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='RankyTools Semrush');
INSERT INTO tools (name, category, limits_json)
SELECT 'RankyTools Majestic', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='RankyTools Majestic');
INSERT INTO tools (name, category, limits_json)
SELECT 'RankyTools ChatGPT', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='RankyTools ChatGPT');
INSERT INTO tools (name, category, limits_json)
SELECT 'RankyTools Ubersuggest', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='RankyTools Ubersuggest');
INSERT INTO tools (name, category, limits_json)
SELECT 'RankyTools Grammarly', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='RankyTools Grammarly');
INSERT INTO tools (name, category, limits_json)
SELECT 'RankyTools Envato Elements', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='RankyTools Envato Elements');
INSERT INTO tools (name, category, limits_json)
SELECT 'RankyTools Storyblocks', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='RankyTools Storyblocks');
INSERT INTO tools (name, category, limits_json)
SELECT 'RankyTools SpyFu', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='RankyTools SpyFu');
INSERT INTO tools (name, category, limits_json)
SELECT 'RankyTools Niche Scraper', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='RankyTools Niche Scraper');
INSERT INTO tools (name, category, limits_json)
SELECT 'RankyTools SpamZilla', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='RankyTools SpamZilla');
INSERT INTO tools (name, category, limits_json)
SELECT 'RankyTools SimilarWeb', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='RankyTools SimilarWeb');

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'RankyTools Semrush', 'ryksm1.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://sr.rankytools.com', 'ryksm1'
FROM tools t WHERE t.name='RankyTools Semrush'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('ryksm1.upays.in','ryksm1.gt4rents.com'))
LIMIT 1;

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'RankyTools Majestic', 'rykmj.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://mj.rankytools.com', 'rykmj'
FROM tools t WHERE t.name='RankyTools Majestic'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('rykmj.upays.in','rykmj.gt4rents.com'))
LIMIT 1;

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'RankyTools ChatGPT', 'rykcp.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://cp.rankytools.com', 'rykcp'
FROM tools t WHERE t.name='RankyTools ChatGPT'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('rykcp.upays.in','rykcp.gt4rents.com'))
LIMIT 1;

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'RankyTools Ubersuggest', 'rykus.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://us.rankytools.com', 'rykus'
FROM tools t WHERE t.name='RankyTools Ubersuggest'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('rykus.upays.in','rykus.gt4rents.com'))
LIMIT 1;

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'RankyTools Grammarly', 'rykgm.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://gm.rankytools.com', 'rykgm'
FROM tools t WHERE t.name='RankyTools Grammarly'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('rykgm.upays.in','rykgm.gt4rents.com'))
LIMIT 1;

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'RankyTools Envato Elements', 'ryken.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://en.rankytools.com', 'ryken'
FROM tools t WHERE t.name='RankyTools Envato Elements'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('ryken.upays.in','ryken.gt4rents.com'))
LIMIT 1;

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'RankyTools Storyblocks', 'ryksb.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://sb.rankytools.com', 'ryksb'
FROM tools t WHERE t.name='RankyTools Storyblocks'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('ryksb.upays.in','ryksb.gt4rents.com'))
LIMIT 1;

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'RankyTools SpyFu', 'ryksf.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://sf.rankytools.com', 'ryksf'
FROM tools t WHERE t.name='RankyTools SpyFu'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('ryksf.upays.in','ryksf.gt4rents.com'))
LIMIT 1;

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'RankyTools Niche Scraper', 'rykns.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://ns.rankytools.com', 'rykns'
FROM tools t WHERE t.name='RankyTools Niche Scraper'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('rykns.upays.in','rykns.gt4rents.com'))
LIMIT 1;

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'RankyTools SpamZilla', 'ryksz02.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://sz02.rankytools.com', 'ryksz02'
FROM tools t WHERE t.name='RankyTools SpamZilla'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('ryksz02.upays.in','ryksz02.gt4rents.com'))
LIMIT 1;

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'RankyTools SimilarWeb', 'ryksw.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://sw.rankytools.com', 'ryksw'
FROM tools t WHERE t.name='RankyTools SimilarWeb'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('ryksw.upays.in','ryksw.gt4rents.com'))
LIMIT 1;

-- Empty active account rows (cookie filled from cookie.json on first tool start, or via Recloud UI).
INSERT INTO accounts (website_id, name, cookie, user_agent_id, user_agent, proxy_id, proxy, status, show_limit, description, automation_task_uid, automation_ingest_key, cookie_updated_at, last_used_at)
SELECT w.id, w.name, '', NULL, '', NULL, '', 'active', 1, '', '', '', UTC_TIMESTAMP(), ''
FROM websites w
WHERE w.domain IN (
  'ryksm1.upays.in','rykmj.upays.in','rykcp.upays.in','rykus.upays.in','rykgm.upays.in',
  'ryken.upays.in','ryksb.upays.in','ryksf.upays.in','rykns.upays.in','ryksz02.upays.in','ryksw.upays.in'
)
AND NOT EXISTS (SELECT 1 FROM accounts a WHERE a.website_id=w.id);

-- Product Mapping: aMember product id 5 (rykcp ChatGPT excluded — unstable gateway).
INSERT INTO products (website_id, product_name, product_ids_json)
SELECT w.id, w.name, '["5"]'
FROM websites w
WHERE w.domain IN (
  'ryksm1.upays.in','rykmj.upays.in','rykus.upays.in','rykgm.upays.in',
  'ryken.upays.in','ryksb.upays.in','ryksf.upays.in','rykns.upays.in','ryksz02.upays.in','ryksw.upays.in'
)
AND NOT EXISTS (SELECT 1 FROM products p WHERE p.website_id=w.id);

UPDATE products p
JOIN websites w ON w.id=p.website_id
SET p.product_ids_json='["5"]', p.product_name=w.name
WHERE w.domain IN (
  'ryksm1.upays.in','rykmj.upays.in','rykus.upays.in','rykgm.upays.in',
  'ryken.upays.in','ryksb.upays.in','ryksf.upays.in','rykns.upays.in','ryksz02.upays.in','ryksw.upays.in'
);
