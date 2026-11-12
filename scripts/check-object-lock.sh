#!/usr/bin/env bash
# Proves the local vault bucket is really immutable: writes an object, then
# tries to delete that exact version with the ROOT admin credentials.
# COMPLIANCE-mode retention must refuse, even for root.
set -euo pipefail
cd "$(dirname "$0")/.."
set -a; source .env; set +a

s3() {
  docker compose -f deploy/compose/compose.yaml --env-file .env run --rm --no-deps -T \
    --entrypoint aws minio-init --endpoint-url http://minio:9000 s3api "$@"
}

key="lock-check/$(date +%s).txt"
version=$(s3 put-object --bucket "$AEGIS_BUCKET" --key "$key" --body /etc/hostname --query VersionId --output text)
echo "wrote $key (version $version)"

if s3 delete-object --bucket "$AEGIS_BUCKET" --key "$key" --version-id "$version" 2>&1; then
  echo "FAIL: root deleted a locked object version - Object Lock is NOT enforced"
  exit 1
fi
echo "PASS: delete refused under COMPLIANCE retention, even for the root user"
