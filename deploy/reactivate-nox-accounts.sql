-- After logout we used to mark accounts logged_out (Temporarily Unavailable).
-- Reactivate any Nox Semrush/Site Audit accounts that still have a cookie.
UPDATE accounts a
JOIN websites w ON w.id=a.website_id
SET a.status='active'
WHERE w.kind='recloud'
  AND (w.domain LIKE 'nxsm%.upays.in' OR w.domain LIKE 'nxaud%.upays.in'
    OR w.domain LIKE 'nxsm%.gt4rents.com' OR w.domain LIKE 'nxaud%.gt4rents.com')
  AND a.status='logged_out'
  AND TRIM(a.cookie) <> '';

SELECT w.domain, a.id, a.status, CHAR_LENGTH(a.cookie) AS cookie_len
FROM accounts a
JOIN websites w ON w.id=a.website_id
WHERE w.domain LIKE 'nxsm%.upays.in' OR w.domain LIKE 'nxaud%.upays.in'
ORDER BY w.domain;
