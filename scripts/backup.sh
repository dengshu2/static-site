#!/bin/sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
backup_dir=${BACKUP_DIR:-"$project_dir/backups"}
retention_days=${BACKUP_RETENTION_DAYS:-7}
timestamp=$(date -u +%Y%m%dT%H%M%SZ)

umask 077
mkdir -p "$backup_dir"
archive="$backup_dir/static-site-data-$timestamp.tar.gz"
tar -C "$project_dir" -czf "$archive" data
sha256sum "$archive" > "$archive.sha256"

find "$backup_dir" -maxdepth 1 -type f -name 'static-site-data-*.tar.gz' -mtime "+$retention_days" -delete
find "$backup_dir" -maxdepth 1 -type f -name 'static-site-data-*.tar.gz.sha256' -mtime "+$retention_days" -delete

printf '%s\n' "$archive"
