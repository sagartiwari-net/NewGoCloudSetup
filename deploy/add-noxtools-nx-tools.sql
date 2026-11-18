-- NoxTools Semrush (nxsm1-5) + Site Audit (nxaud1-3) as panel recloud websites.
-- Each site has its own accounts row so cookies update separately in Recloud UI.
-- session_security_enabled=1 → panel access link + device lock.
-- Run on MySQL gt4rents after code deploy (or rely on ensureRecloudSite on first start).

-- Semrush 1..5
INSERT INTO tools (name, category, limits_json)
SELECT 'NoxTools Semrush 1', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='NoxTools Semrush 1');
INSERT INTO tools (name, category, limits_json)
SELECT 'NoxTools Semrush 2', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='NoxTools Semrush 2');
INSERT INTO tools (name, category, limits_json)
SELECT 'NoxTools Semrush 3', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='NoxTools Semrush 3');
INSERT INTO tools (name, category, limits_json)
SELECT 'NoxTools Semrush 4', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='NoxTools Semrush 4');
INSERT INTO tools (name, category, limits_json)
SELECT 'NoxTools Semrush 5', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='NoxTools Semrush 5');

INSERT INTO tools (name, category, limits_json)
SELECT 'NoxTools Site Audit 1', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='NoxTools Site Audit 1');
INSERT INTO tools (name, category, limits_json)
SELECT 'NoxTools Site Audit 2', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='NoxTools Site Audit 2');
INSERT INTO tools (name, category, limits_json)
SELECT 'NoxTools Site Audit 3', 'recloud', '[]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='NoxTools Site Audit 3');

-- Helper pattern: insert website if domain missing
INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'NoxTools Semrush 1', 'nxsm1.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://1.semrush.com.in', 'nxsm1'
FROM tools t WHERE t.name='NoxTools Semrush 1'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('nxsm1.upays.in','nxsm1.gt4rents.com'))
LIMIT 1;

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'NoxTools Semrush 2', 'nxsm2.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://2.semrush.com.in', 'nxsm2'
FROM tools t WHERE t.name='NoxTools Semrush 2'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('nxsm2.upays.in','nxsm2.gt4rents.com'))
LIMIT 1;

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'NoxTools Semrush 3', 'nxsm3.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://3.semrush.com.in', 'nxsm3'
FROM tools t WHERE t.name='NoxTools Semrush 3'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('nxsm3.upays.in','nxsm3.gt4rents.com'))
LIMIT 1;

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'NoxTools Semrush 4', 'nxsm4.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://4.semrush.com.in', 'nxsm4'
FROM tools t WHERE t.name='NoxTools Semrush 4'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('nxsm4.upays.in','nxsm4.gt4rents.com'))
LIMIT 1;

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'NoxTools Semrush 5', 'nxsm5.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://5.semrush.com.in', 'nxsm5'
FROM tools t WHERE t.name='NoxTools Semrush 5'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('nxsm5.upays.in','nxsm5.gt4rents.com'))
LIMIT 1;

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'NoxTools Site Audit 1', 'nxaud1.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://siteaudit1.semrush.com.in', 'nxaud1'
FROM tools t WHERE t.name='NoxTools Site Audit 1'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('nxaud1.upays.in','nxaud1.gt4rents.com'))
LIMIT 1;

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'NoxTools Site Audit 2', 'nxaud2.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://siteaudit2.semrush.com.in', 'nxaud2'
FROM tools t WHERE t.name='NoxTools Site Audit 2'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('nxaud2.upays.in','nxaud2.gt4rents.com'))
LIMIT 1;

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, kind, upstream_url, access_slug)
SELECT t.id, 'NoxTools Site Audit 3', 'nxaud3.upays.in', REPLACE(UUID(),'-',''), 30, '{}', 1, 'recloud', 'https://siteaudit3.semrush.com.in', 'nxaud3'
FROM tools t WHERE t.name='NoxTools Site Audit 3'
  AND NOT EXISTS (SELECT 1 FROM websites WHERE domain IN ('nxaud3.upays.in','nxaud3.gt4rents.com'))
LIMIT 1;

-- Empty active account rows (cookie filled from cookie.json on first tool start, or via Recloud UI).
INSERT INTO accounts (website_id, name, cookie, user_agent_id, user_agent, proxy_id, proxy, status, show_limit, description, automation_task_uid, automation_ingest_key, cookie_updated_at, last_used_at)
SELECT w.id, w.name, '', NULL, '', NULL, '', 'active', 1, '', '', '', UTC_TIMESTAMP(), ''
FROM websites w
WHERE w.domain IN (
  'nxsm1.upays.in','nxsm2.upays.in','nxsm3.upays.in','nxsm4.upays.in','nxsm5.upays.in',
  'nxaud1.upays.in','nxaud2.upays.in','nxaud3.upays.in'
)
AND NOT EXISTS (SELECT 1 FROM accounts a WHERE a.website_id=w.id);
