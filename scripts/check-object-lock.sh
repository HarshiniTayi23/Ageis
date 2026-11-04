#!/usr/bin/env bash
# Proves the local vault bucket is really immutable: writes an object, then
# tries to delete that exact version with the ROOT admin credentials.
# COMPLIANCE-mode retention must refuse, even for root.
set -euo pipefail
cd "$(dirname "$0")/.."
set -a; source .env; set +a

mc() {
  docker compose -f deploy/compose/compose.yaml --env-file .env run --rm --no-deps \
    --entrypoint /bin/sh minio-init -c \
    "mc alias set local http://minio:9000 \"\$MINIO_ROOT_USER\" \"\$MINIO_ROOT_PASSWORD\" >/dev/null && $*"
}

key="lock-check/$(date +%s).txt"
mc "echo immutable | mc pipe local/$AEGIS_BUCKET/$key"
version=$(mc "mc stat --json local/$AEGIS_BUCKET/$key" | grep -o '"versionID":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "wrote $key (version $version)"

if mc "mc rm --version-id $version local/$AEGIS_BUCKET/$key" 2>&1; then
  echo "FAIL: root deleted a locked object version - Object Lock is NOT enforced"
  exit 1
fi
echo "PASS: delete refused under COMPLIANCE retention, even for the root user"
