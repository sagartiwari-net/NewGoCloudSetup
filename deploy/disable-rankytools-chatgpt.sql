-- Take RankyTools ChatGPT (rykcp) offline from Access / aMember product 5.
-- Keeps the website row for history; clears product mapping and marks the name OFF.
-- Binary/code stays in repo — do not re-map until ChatGPT gateway is stable.

UPDATE products p
JOIN websites w ON w.id = p.website_id
SET p.product_ids_json = '[]',
    p.product_name = CONCAT(w.name, ' (OFF)')
WHERE w.domain IN ('rykcp.upays.in', 'rykcp.gt4rents.com')
   OR w.access_slug = 'rykcp';

UPDATE websites
SET name = 'RankyTools ChatGPT (OFF)'
WHERE domain IN ('rykcp.upays.in', 'rykcp.gt4rents.com')
   OR access_slug = 'rykcp';

UPDATE accounts a
JOIN websites w ON w.id = a.website_id
SET a.status = 'inactive'
WHERE w.domain IN ('rykcp.upays.in', 'rykcp.gt4rents.com')
   OR w.access_slug = 'rykcp';

DELETE l FROM account_cookie_links l
JOIN accounts a ON a.id = l.follower_account_id
JOIN websites w ON w.id = a.website_id
WHERE w.domain IN ('rykcp.upays.in', 'rykcp.gt4rents.com')
   OR w.access_slug = 'rykcp';

SELECT w.id, w.name, w.domain, w.access_slug, p.product_ids_json, a.status AS account_status
FROM websites w
LEFT JOIN products p ON p.website_id = w.id
LEFT JOIN accounts a ON a.website_id = w.id
WHERE w.domain LIKE 'rykcp.%' OR w.access_slug = 'rykcp';
