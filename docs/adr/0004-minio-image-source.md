# ADR-0004: Pull MinIO from Quay after its removal from Docker Hub

**Status:** Accepted · **Date:** 2026-11-12 · Follows up on [ADR-0003](0003-go-and-minio-object-lock.md) · Superseded by [ADR-0005](0005-chainguard-minio-and-aws-cli.md)

## Context
The first CI run failed with `pull access denied for minio/minio`. Around 12 September 2026 Docker Hub stopped serving the `minio/minio` and `minio/mc` repositories. This came at the end of a longer change upstream: MinIO Community Edition went source-only in late 2025, and the repository was archived in April 2026.

Our CI had never pulled the image before this run, so the break was only discovered on the first push. A dependency can vanish even when its version is pinned.

## Decision
Use `quay.io/minio/minio:RELEASE.2025-09-07T16-13-09Z`, a historical release that is still on Quay. The image bundles the `mc` client, so the bucket-setup job uses the same image.

## Options considered
| Option | Pros | Cons |
|---|---|---|
| **Quay, pinned release (chosen)** | Same binary as before, Object Lock behaves identically, pinned tag, one-line change | Upstream is archived, so no security fixes; Quay may also stop serving it |
| `cgr.dev/chainguard/minio` | Rebuilt from source and patched | Free tier only offers `latest`, so not reproducible; `mc` not bundled |
| `pgsty/minio` community fork | Actively maintained, drop-in | Small maintainer base; trust and longevity unclear |
| Switch to Garage or SeaweedFS | Actively developed | Object Lock COMPLIANCE support is less complete; Object Lock is the core of our security claim |
| Real AWS S3 in CI | The real target, fully supported | Needs cloud credentials in CI and costs money |

## Consequences
- This is acceptable because MinIO only runs in local development and CI, never in production. The production target is AWS S3 Object Lock.
- Because the storage code talks to the S3 API, not to MinIO specifically, swapping MinIO for any S3-compatible server later is a configuration change, not a rewrite.
- **Revisit if:** the Quay image disappears, or the M1 S3 backend needs features this release lacks. The first fallback is the Chainguard image.
