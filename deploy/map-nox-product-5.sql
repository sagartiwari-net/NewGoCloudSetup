-- Map aMember product id 5 onto all Nox Semrush + Site Audit recloud websites.
-- Run on server:
--   source /www/wwwroot/gt4rents.com/_secrets/mysql.env
--   mysql -ugt4rents -p"$GT4RENTS_MYSQL_PASSWORD" gt4rents < deploy/map-nox-product-5.sql

INSERT INTO products (website_id, product_name, product_ids_json)
SELECT w.id, w.name, '["5"]'
FROM websites w
WHERE w.domain IN (
  'nxsm1.upays.in','nxsm2.upays.in','nxsm3.upays.in','nxsm4.upays.in','nxsm5.upays.in',
  'nxaud1.upays.in','nxaud2.upays.in','nxaud3.upays.in'
)
AND NOT EXISTS (SELECT 1 FROM products p WHERE p.website_id=w.id);

UPDATE products p
JOIN websites w ON w.id=p.website_id
SET p.product_ids_json='["5"]', p.product_name=w.name
WHERE w.domain IN (
  'nxsm1.upays.in','nxsm2.upays.in','nxsm3.upays.in','nxsm4.upays.in','nxsm5.upays.in',
  'nxaud1.upays.in','nxaud2.upays.in','nxaud3.upays.in'
);

SELECT w.domain, p.product_name, p.product_ids_json
FROM websites w
LEFT JOIN products p ON p.website_id=w.id
WHERE w.domain LIKE 'nxsm%.upays.in' OR w.domain LIKE 'nxaud%.upays.in'
ORDER BY w.domain;
