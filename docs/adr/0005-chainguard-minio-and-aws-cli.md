# ADR-0005: Chainguard MinIO for the server, AWS CLI for bucket setup and checks

**Status:** Accepted · **Date:** 2026-11-12 · Supersedes [ADR-0004](0004-minio-image-source.md)

## Context
ADR-0004 moved to `quay.io/minio/minio`. Its first CI run failed: Quay now answers `401 UNAUTHORIZED` for that image. Both of MinIO's own public registries are gone within two months of each other. This is exactly the "revisit if" condition ADR-0004 named.

## Decision
- **Server:** `cgr.dev/chainguard/minio:latest`, Chainguard's patched rebuild of MinIO from source, run as root so it can write its volume.
- **Bucket setup and the Object Lock check:** `amazon/aws-cli:2.17.0` instead of MinIO's `mc` tool, which Chainguard doesn't bundle.

## Options considered
| Option | Pros | Cons |
|---|---|---|
| **Chainguard + AWS CLI (chosen)** | Maintained and patched; the check now uses the same S3 API calls Aegis will make against AWS | Free tier only offers `latest`, so builds are not fully reproducible |
| Build MinIO from the archived source in our own Dockerfile | Fully pinned and reproducible | Slow CI; we'd own patching an archived codebase |
| `pgsty/minio` community fork | Drop-in replacement | Small maintainer base, trust unclear |
| Garage or SeaweedFS | Actively developed | Weaker Object Lock COMPLIANCE support, which is our core claim |

## Consequences
- Dropping `mc` removes a MinIO-specific dependency: `scripts/check-object-lock.sh` would now work unchanged against real AWS S3.
- Because `latest` can change under us, the CI Object Lock check is the safety net. If a new Chainguard build breaks Object Lock, CI fails loudly.
- **Lesson recorded:** pinning a version protects you from *changes*, not from *disappearance*. Production would mirror critical images into a registry we control.
- **Revisit if:** the Chainguard free tier is withdrawn. Next step: build from archived source.
