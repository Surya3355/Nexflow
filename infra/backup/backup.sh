#!/usr/bin/env bash
# Daily backup: a PostgreSQL dump and a copy of the uploaded files, then
# retention (30 days kept, spec §12). Runs from cron at 02:30 IST, after the
# application's 02:00 purge.
#
# Needs: NEXFLOW_DB_URL, NEXFLOW_BACKUP_DIR, NEXFLOW_UPLOAD_DIR, and pg_dump.
# The off-server copy to a second location in India is hosting-blocked and is
# not done here.
set -euo pipefail

: "${NEXFLOW_DB_URL:?set NEXFLOW_DB_URL}"
: "${NEXFLOW_BACKUP_DIR:?set NEXFLOW_BACKUP_DIR}"
upload_dir=${NEXFLOW_UPLOAD_DIR:-uploads}
here=$(cd "$(dirname "$0")" && pwd)
day=$(TZ=Asia/Kolkata date +%F)

mkdir -p "$NEXFLOW_BACKUP_DIR"
pg_dump --format=custom --file "$NEXFLOW_BACKUP_DIR/nexflow-$day.dump" "$NEXFLOW_DB_URL"

if [ -d "$upload_dir" ]; then
  tar -czf "$NEXFLOW_BACKUP_DIR/files-$day.tar.gz" -C "$upload_dir" .
fi

"$here/retain.sh" "$NEXFLOW_BACKUP_DIR" 30
echo "backup: done for $day"
