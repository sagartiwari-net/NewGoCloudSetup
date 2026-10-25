-- Veo3 off the panel (both hosts). WriteHuman stays on wrth.upays.in.
DELETE FROM websites
 WHERE name = 'Veo3 AI'
    OR domain LIKE 'veo3.%'
    OR domain = 'ebfb.upays.in';

DELETE FROM websites
 WHERE domain = 'writehuman.gt4rents.com';

DELETE FROM tools
 WHERE name = 'Veo3 AI'
   AND id NOT IN (SELECT tool_id FROM websites);
