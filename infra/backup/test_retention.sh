#!/usr/bin/env bash
# Plan Task 33 test: 35 dated dummy backups must leave exactly the 30 newest.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
# A fresh temporary folder, so the test also runs where the repo is read-only.
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT

for i in $(seq 0 34); do
  d=$(date -d "2026-10-01 +$i day" +%F)
  : > "$dir/nexflow-$d.dump"
  : > "$dir/files-$d.tar.gz"
done

"$here/retain.sh" "$dir" 30 >/dev/null

dumps=$(find "$dir" -maxdepth 1 -name 'nexflow-*.dump' | wc -l)
files=$(find "$dir" -maxdepth 1 -name 'files-*.tar.gz' | wc -l)
oldest_kept=$(find "$dir" -maxdepth 1 -name 'nexflow-*.dump' -printf '%f\n' | sort | head -1)
newest_kept=$(find "$dir" -maxdepth 1 -name 'nexflow-*.dump' -printf '%f\n' | sort | tail -1)

[ "$dumps" -eq 30 ] || { echo "FAIL: dumps kept = $dumps, want 30" >&2; exit 1; }
[ "$files" -eq 30 ] || { echo "FAIL: file archives kept = $files, want 30" >&2; exit 1; }
[ "$newest_kept" = "nexflow-2026-11-04.dump" ] || { echo "FAIL: newest kept = $newest_kept" >&2; exit 1; }
[ "$oldest_kept" = "nexflow-2026-10-06.dump" ] || { echo "FAIL: oldest kept = $oldest_kept" >&2; exit 1; }
echo "PASS: 35 dumps and 35 archives -> 30 each, newest kept"
