# ADR-0003: Go for the service, MinIO with Object Lock for local immutable storage

**Status:** Accepted · **Date:** 2026-11-04

## Context
We need a language suited to concurrent I/O-heavy services, and a local storage backend that is *actually* immutable. The current local-disk store can be overwritten by root, which the demo shows openly.

## Decision
- **Go** for all service code.
- **MinIO** (S3-compatible) in Docker for local development, with a bucket created with **Object Lock** and default **COMPLIANCE** retention. The same code then targets AWS S3 Object Lock in the cloud.

## Options considered
| Option | Pros | Cons |
|---|---|---|
| **Go** | Goroutines and channels fit worker pools; static binaries make tiny containers; widely used in cloud infrastructure, including at Rubrik | Less expressive than Scala; simple generics |
| Python | Fastest to write | Weak at CPU-bound concurrency (GIL); harder to ship as a small static image |
| Rust | Strongest safety and speed | Slowest to learn in a short timeline |
| Local disk only | No dependencies | Root can delete anything, so it isn't real immutability |
| **MinIO + Object Lock** | Free, runs in Docker, same S3 API as AWS, COMPLIANCE mode refuses deletes even for root | Community edition no longer gets new Docker images (late 2025), so we pin a release |
| LocalStack | Emulates many AWS services | Object Lock enforcement is weaker than MinIO's |

## Consequences
- `scripts/check-object-lock.sh` proves in CI that the root user cannot delete a locked object.
- A future ADR will decide what replaces MinIO if the pinned image stops being usable.
