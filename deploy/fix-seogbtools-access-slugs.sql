-- Button codes for *.seogbtools.com: same as upays.in, with sgb prefix.
-- refs → sgbrefs, cnva → sgbcnva. Strips any -2 collision suffix from the upays slug.

UPDATE websites sgb
INNER JOIN websites up
        ON up.domain = REPLACE(sgb.domain, '.seogbtools.com', '.upays.in')
   SET sgb.access_slug = CONCAT(
         'sgb',
         IF(
           TRIM(COALESCE(up.access_slug, '')) = '',
           SUBSTRING_INDEX(sgb.domain, '.', 1),
           SUBSTRING_INDEX(up.access_slug, '-', 1)
         )
       )
 WHERE sgb.domain LIKE '%.seogbtools.com';
