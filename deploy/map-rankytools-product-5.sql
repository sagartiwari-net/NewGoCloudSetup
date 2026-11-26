-- One-shot: map all RankyTools recloud sites to aMember product id 5.
INSERT INTO products (website_id, product_name, product_ids_json)
SELECT w.id, w.name, '["5"]'
FROM websites w
WHERE w.domain IN (
  'ryksm1.upays.in','rykmj.upays.in','rykcp.upays.in','rykus.upays.in','rykgm.upays.in',
  'ryken.upays.in','ryksb.upays.in','ryksf.upays.in','rykns.upays.in','ryksz02.upays.in','ryksw.upays.in'
)
AND NOT EXISTS (SELECT 1 FROM products p WHERE p.website_id=w.id);

UPDATE products p
JOIN websites w ON w.id=p.website_id
SET p.product_ids_json='["5"]', p.product_name=w.name
WHERE w.domain IN (
  'ryksm1.upays.in','rykmj.upays.in','rykcp.upays.in','rykus.upays.in','rykgm.upays.in',
  'ryken.upays.in','ryksb.upays.in','ryksf.upays.in','rykns.upays.in','ryksz02.upays.in','ryksw.upays.in'
);

SELECT w.domain, p.product_ids_json
FROM websites w
JOIN products p ON p.website_id=w.id
WHERE w.domain LIKE 'ryk%.upays.in'
ORDER BY w.domain;
