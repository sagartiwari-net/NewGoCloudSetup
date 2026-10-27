-- Prefix SeoGroupBuy on friend *.seogbtools.com website names. Does not touch upays.in.

UPDATE websites
   SET name = CONCAT('SeoGroupBuy ', name)
 WHERE domain LIKE '%.seogbtools.com'
   AND name NOT LIKE 'SeoGroupBuy %';

UPDATE products p
  JOIN websites w ON w.id = p.website_id
   SET p.product_name = w.name
 WHERE w.domain LIKE '%.seogbtools.com'
   AND p.product_name NOT LIKE 'SeoGroupBuy %';

UPDATE accounts a
  JOIN websites w ON w.id = a.website_id
   SET a.name = CONCAT('SeoGroupBuy ', a.name)
 WHERE w.domain LIKE '%.seogbtools.com'
   AND a.name NOT LIKE 'SeoGroupBuy %';
