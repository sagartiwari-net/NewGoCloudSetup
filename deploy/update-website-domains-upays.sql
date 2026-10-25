-- Live MySQL `gt4rents`. Friend hosts without gt4rents.com are untouched.
-- phpMyAdmin → SQL, or: mysql -u gt4rents -p gt4rents < this file

-- 1) Panel dropdown + Open links: owner tools → *.upays.in
UPDATE websites
   SET domain = REPLACE(domain, 'gt4rents.com', 'upays.in')
 WHERE domain LIKE '%gt4rents.com';

-- 2) Speechify off the panel (phishing hold)
DELETE FROM websites
 WHERE domain LIKE 'speechify.%'
    OR name = 'Speechify';

DELETE FROM tools
 WHERE name = 'Speechify'
   AND id NOT IN (SELECT tool_id FROM websites);

-- Check
SELECT id, name, domain FROM websites WHERE domain LIKE '%gt4rents.com';
SELECT id, name, domain FROM websites WHERE domain LIKE 'speechify.%' OR name = 'Speechify';
SELECT id, name, domain FROM websites ORDER BY name LIMIT 20;
