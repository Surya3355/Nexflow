-- Files the purge job could not delete after their rows were removed (round
-- 16 review). The next purge tries them again, so customer documents never
-- stay on disk past their retention date unnoticed.
CREATE TABLE pending_file_deletes (
  storage_key text PRIMARY KEY,
  failed_at   timestamptz NOT NULL DEFAULT now(),
  last_error  text NOT NULL
);
