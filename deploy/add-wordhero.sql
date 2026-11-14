-- WordHero owner site (wh.upays.in → port 5322)
-- Run on MySQL gt4rents if panel seed has not created the row yet.

INSERT INTO tools (name, category, limits_json)
SELECT 'WordHero', 'ai', '[{"key":"credits","label":"Credits","default":100}]'
WHERE NOT EXISTS (SELECT 1 FROM tools WHERE name='WordHero');

INSERT INTO websites (tool_id, name, domain, secret_key, session_duration, default_limits_json, session_security_enabled, access_slug)
SELECT t.id, 'WordHero', 'wh.upays.in', 'secret_wordhero', 120, '{"credits":100}', 1, 'wh'
FROM tools t
WHERE t.name='WordHero'
  AND NOT EXISTS (
    SELECT 1 FROM websites
    WHERE name='WordHero' OR domain IN ('wh.upays.in','wordhero.gt4rents.com','wordhero.upays.in')
  )
LIMIT 1;

INSERT INTO products (website_id, product_name, product_ids_json)
SELECT w.id, 'WordHero', '["wordhero"]'
FROM websites w
WHERE w.domain='wh.upays.in'
  AND NOT EXISTS (SELECT 1 FROM products WHERE website_id=w.id AND product_name='WordHero')
LIMIT 1;
