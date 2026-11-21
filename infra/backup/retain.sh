#!/usr/bin/env bash
# Keep the newest N backups of each kind in a directory; delete the rest.
# Backups are named with their date (YYYY-MM-DD), so name order is date order.
#
# Usage: retain.sh BACKUP_DIR [KEEP]   (KEEP defaults to 30, spec §12)
set -euo pipefail

dir=${1:?usage: retain.sh BACKUP_DIR [KEEP]}
keep=${2:-30}

[ -d "$dir" ] || { echo "retain: not a directory: $dir" >&2; exit 2; }

prune() {
  local pattern=$1
  # Newest first; everything after the first KEEP lines is removed.
  local old
  old=$(find "$dir" -maxdepth 1 -type f -name "$pattern" -printf '%f\n' | sort -r | tail -n +"$((keep + 1))")
  while IFS= read -r name; do
    [ -n "$name" ] || continue
    rm -f -- "$dir/$name"
    echo "retain: removed $name"
  done <<< "$old"
}

prune 'nexflow-*.dump'
prune 'files-*.tar.gz'
