# ADR-0006: An S3 client on the Go standard library, with write-once semantics from conditional writes

**Status:** Accepted · **Date:** 2026-11-12

## Context
R6 in the PRD needs an S3 backend whose guarantees match the `ObjectStore` contract: `Put` never overwrites, and `Delete` fails during retention. The obvious choice, the AWS SDK for Go v2, couldn't be fetched in the development environment: the Go module mirror was unreachable, and fetching the SDK's monorepo directly from GitHub timed out. We needed six S3 operations, not the whole SDK.

## Decision
1. **Client:** about 300 lines on `net/http`: Put, Get, Head, List (v2, paginated), and version-specific Delete, plus our own **Signature V4** signer.
2. **Signer correctness:** the signer is tested against **AWS's four published SigV4 examples** (GET object, PUT object with `$` in the key, subresource query, list), and every request in the unit tests is re-verified server-side by a fake S3.
3. **Write-once:** `Put` sends `If-None-Match: *` (S3 conditional write). A 412 response maps to `ErrExists`. No read-then-write race.
4. **Retention:** every `Put` sets `x-amz-object-lock-mode: COMPLIANCE` and a retain-until date, on top of the bucket's default retention. That's defence in depth: if someone misconfigures the bucket default, objects are still locked.
5. **Delete:** resolve the current version ID with HEAD, then delete *that version*. Under retention S3 refuses with 403 on AWS or 400 on MinIO, which maps to `ErrImmutable`.

## Options considered
| Option | Pros | Cons |
|---|---|---|
| **Own client on stdlib (chosen)** | Zero dependencies; every line is understood and tested; small attack surface | We own retries, edge cases and future S3 changes; no SSO/IMDS credential discovery |
| AWS SDK for Go v2 | Battle-tested; every auth method | Huge dependency tree; couldn't be fetched here; hides how S3 actually works |
| minio-go | Smaller than the AWS SDK | Still a dependency, and tied to a project that was just archived |

## Known gaps (accepted for M1)
- **Delete markers:** on a versioned bucket, anyone with write access can add a delete marker that *hides* the latest version from plain GETs. The locked data survives, but `Get` returns not-found. A recovery path that lists versions, plus an IAM policy denying `s3:DeleteObject` to backup writers, is roadmap work.
- **Retried PUT after a lost response:** the retry gets 412 and reports `ErrExists`. For chunks that's correct (same content). For a snapshot manifest the backup would fail and need a re-run. That's rare and safe, because nothing is lost.
- **Credentials:** only static keys from the environment. IAM roles on EKS/EC2 would need IMDS or IRSA support.

## Consequences
- `make integration` runs a full backup → attacker → detect → restore → drill cycle against real MinIO in CI, using the MinIO **root** credentials for the "attacker".
- Swapping to the SDK later only touches `internal/store/s3.go`.
