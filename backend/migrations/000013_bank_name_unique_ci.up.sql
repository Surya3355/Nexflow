-- Bank names are unique ignoring capitals (round 16, spec 2026-11-27-hardening §5).
-- Existing duplicates are not changed here: the migration stops and names
-- them, so they are renamed by hand first.
DO $$
DECLARE dups text;
BEGIN
  SELECT string_agg(format('%s (ids %s)', lname, ids), '; ') INTO dups
  FROM (SELECT lower(name) AS lname, string_agg(id::text, ', ' ORDER BY id) AS ids
        FROM banks GROUP BY lower(name) HAVING count(*) > 1) d;
  IF dups IS NOT NULL THEN
    RAISE EXCEPTION 'bank names that differ only in capitals: %. Rename one of each pair, then start again.', dups;
  END IF;
END
$$;

CREATE UNIQUE INDEX banks_name_ci_uidx ON banks (lower(name));
