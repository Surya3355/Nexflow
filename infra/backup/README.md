# Nexflow backups

Daily backup of the database and uploaded files, kept for 30 days (spec §12).

- `backup.sh` writes `nexflow-YYYY-MM-DD.dump` (pg_dump, custom format) and
  `files-YYYY-MM-DD.tar.gz` (uploaded files), then runs retention.
- `retain.sh DIR [KEEP]` keeps the newest KEEP of each kind (default 30).
- `test_retention.sh` checks retention with 35 dated dummy backups.

Set `NEXFLOW_DB_URL`, `NEXFLOW_BACKUP_DIR`, `NEXFLOW_UPLOAD_DIR`. The
database role that runs pg_dump needs read access to all tables.

Cron, on the server (IST, after the 02:00 application purge):

    30 2 * * * /path/to/infra/backup/backup.sh

**Not done yet (hosting-blocked):** the copy to a second location in India.
Until then, backups stay on the server. Restore is not yet tested: run a restore
of the dump into a fresh database and compare row counts (plan Task 33
checkpoint) once pg_dump is available on the host.
