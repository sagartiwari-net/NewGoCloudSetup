-- AmzPremiumToolz extra SellerAmp (seller2–10) on *.amzpremiumsoftware.com.
-- Same Go port as seller (5161). Separate access_slug each. Product id 5. No accounts.
-- Also sets product ["5"] on base seller.amzpremiumsoftware.com.

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
        SELECT 'SellerAmp' tool_name, 'AmzPremium SellerAmp 2' name, 'seller2.amzpremiumsoftware.com' domain, 'seller.amzpremiumsoftware.com' owner, 'amzsllr2' slug
        UNION ALL SELECT 'SellerAmp', 'AmzPremium SellerAmp 3', 'seller3.amzpremiumsoftware.com', 'seller.amzpremiumsoftware.com', 'amzsllr3'
        UNION ALL SELECT 'SellerAmp', 'AmzPremium SellerAmp 4', 'seller4.amzpremiumsoftware.com', 'seller.amzpremiumsoftware.com', 'amzsllr4'
        UNION ALL SELECT 'SellerAmp', 'AmzPremium SellerAmp 5', 'seller5.amzpremiumsoftware.com', 'seller.amzpremiumsoftware.com', 'amzsllr5'
        UNION ALL SELECT 'SellerAmp', 'AmzPremium SellerAmp 6', 'seller6.amzpremiumsoftware.com', 'seller.amzpremiumsoftware.com', 'amzsllr6'
        UNION ALL SELECT 'SellerAmp', 'AmzPremium SellerAmp 7', 'seller7.amzpremiumsoftware.com', 'seller.amzpremiumsoftware.com', 'amzsllr7'
        UNION ALL SELECT 'SellerAmp', 'AmzPremium SellerAmp 8', 'seller8.amzpremiumsoftware.com', 'seller.amzpremiumsoftware.com', 'amzsllr8'
        UNION ALL SELECT 'SellerAmp', 'AmzPremium SellerAmp 9', 'seller9.amzpremiumsoftware.com', 'seller.amzpremiumsoftware.com', 'amzsllr9'
        UNION ALL SELECT 'SellerAmp', 'AmzPremium SellerAmp 10', 'seller10.amzpremiumsoftware.com', 'seller.amzpremiumsoftware.com', 'amzsllr10'
       ) v
  JOIN tools t ON t.name = v.tool_name
  LEFT JOIN websites src ON src.domain = v.owner
 WHERE NOT EXISTS (SELECT 1 FROM websites x WHERE x.domain = v.domain)
   AND NOT EXISTS (SELECT 1 FROM websites x WHERE x.access_slug = v.slug);

INSERT INTO products (website_id, product_name, product_ids_json)
SELECT w.id, w.name, '["5"]'
  FROM websites w
 WHERE w.domain IN (
       'seller2.amzpremiumsoftware.com',
       'seller3.amzpremiumsoftware.com',
       'seller4.amzpremiumsoftware.com',
       'seller5.amzpremiumsoftware.com',
       'seller6.amzpremiumsoftware.com',
       'seller7.amzpremiumsoftware.com',
       'seller8.amzpremiumsoftware.com',
       'seller9.amzpremiumsoftware.com',
       'seller10.amzpremiumsoftware.com'
     )
   AND NOT EXISTS (SELECT 1 FROM products p WHERE p.website_id = w.id);

-- Base + extras: product mapped = 5
UPDATE products p
  JOIN websites w ON w.id = p.website_id
   SET p.product_ids_json = '["5"]'
 WHERE w.domain = 'seller.amzpremiumsoftware.com'
    OR w.domain REGEXP '^seller[0-9]+\\.amzpremiumsoftware\\.com$';

INSERT IGNORE INTO operator_websites (operator_id, website_id)
SELECT o.id, w.id
  FROM operators o
  JOIN websites w ON w.domain IN (
       'seller2.amzpremiumsoftware.com',
       'seller3.amzpremiumsoftware.com',
       'seller4.amzpremiumsoftware.com',
       'seller5.amzpremiumsoftware.com',
       'seller6.amzpremiumsoftware.com',
       'seller7.amzpremiumsoftware.com',
       'seller8.amzpremiumsoftware.com',
       'seller9.amzpremiumsoftware.com',
       'seller10.amzpremiumsoftware.com'
     )
 WHERE o.username = 'AmzPremiumToolz';

INSERT INTO telegram_routes (website_id, events_json)
SELECT w.id, '["logout","spam"]'
  FROM websites w
 WHERE w.domain IN (
       'seller2.amzpremiumsoftware.com',
       'seller3.amzpremiumsoftware.com',
       'seller4.amzpremiumsoftware.com',
       'seller5.amzpremiumsoftware.com',
       'seller6.amzpremiumsoftware.com',
       'seller7.amzpremiumsoftware.com',
       'seller8.amzpremiumsoftware.com',
       'seller9.amzpremiumsoftware.com',
       'seller10.amzpremiumsoftware.com'
     )
   AND NOT EXISTS (SELECT 1 FROM telegram_routes r WHERE r.website_id = w.id);

INSERT IGNORE INTO telegram_route_chats (route_id, destination_id)
SELECT r.id, d.id
  FROM telegram_routes r
  JOIN websites w ON w.id = r.website_id
  JOIN operators o ON o.username = 'AmzPremiumToolz'
  JOIN telegram_destinations d ON d.reseller_id = o.id AND d.enabled = 1
 WHERE w.domain IN (
       'seller2.amzpremiumsoftware.com',
       'seller3.amzpremiumsoftware.com',
       'seller4.amzpremiumsoftware.com',
       'seller5.amzpremiumsoftware.com',
       'seller6.amzpremiumsoftware.com',
       'seller7.amzpremiumsoftware.com',
       'seller8.amzpremiumsoftware.com',
       'seller9.amzpremiumsoftware.com',
       'seller10.amzpremiumsoftware.com'
     );
