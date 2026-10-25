-- Drop leftover *.gt4rents.com website rows (duplicates of live *.upays.in).
-- Friend hosts without gt4rents.com are untouched.
DELETE FROM websites WHERE domain LIKE '%.gt4rents.com';

-- Session length 120 → 30 on every remaining site that still has 120.
UPDATE websites SET session_duration = 30 WHERE session_duration = 120;
