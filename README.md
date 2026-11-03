# Aegis — ransomware-resilient backup that assumes the attacker comes for the backups first

> Encrypted, deduplicated, write-once backups with built-in ransomware detection and verified clean-point recovery. Written in Go; ships as a container, deploys to Kubernetes.

```
$ ./scripts/demo.sh
...
==> 7. Next scheduled backup runs - and the detector fires
!!! RANSOMWARE INDICATORS - anomaly score 0.95 !!!
   - 80% of existing files jumped to near-random entropy
   - 48 files re-appeared with ransomware-style extensions
   - 1 probable ransom-note files
==> 9. Verify repository integrity
  PROBLEM: snapshots/1793…b7edc1: manifest failed authentication (tampered)
==> 10. Recover the latest CLEAN point in time
restored 1793…b8c642 (every file SHA-256 verified)   recovered files: 60  (.locked files: 0)
```

---

## The problem

On **21 February 2024**, the ALPHV/BlackCat ransomware group got into Change Healthcare, a company that sits in the middle of roughly 15 billion US healthcare transactions a year. Pharmacies could not fill prescriptions. Clinics could not bill. Some providers were losing on the order of $100M a day. UnitedHealth paid a reported **$22 million** ransom, then faced a *second* extortion demand for the same stolen data. Total cost is projected at **$2.3–2.45 billion**, and the data of up to **one third of Americans** was exposed.

The uncomfortable lesson is not "they should have had backups." Large organisations *have* backups. The lesson is that **modern ransomware is designed to destroy recovery before it destroys production**:

| What industry surveys found | Source |
|---|---|
| Attackers tried to compromise backups in **93–94%** of ransomware incidents | Veeam (≈3,000 attacks surveyed); Sophos / Vanson Bourne |
| They **succeeded** in crippling the victim's ability to recover in **75%** of those | Veeam |
| Victims whose backups were compromised spent **~8×** more on total recovery | Sophos |
| Only **26%** of them fully recovered within a week (vs. 46% with intact backups) | Sophos |

A backup system built for disk failures has three blind spots that an adversary exploits:

1. **The backups are mutable.** If the backup server's admin credentials are stolen, the attacker deletes or encrypts the backups, then encrypts production. Now there is nothing to restore.
2. **The backups faithfully copy the attack.** Backups run on a schedule. If encryption started at 02:00 and the backup ran at 03:00, the newest backup *is ransomware*. Restoring "the latest" re-infects you, and finding the last good point by hand across thousands of servers takes days.
3. **Nobody verifies the backups can actually be restored** until the worst day of the year.

### Problem statement

> **Design and build a backup service that keeps working when the attacker is already inside the network.** It must:
>
> - **R1 — Survive the attacker.** Backup data must be append-only: no API path can overwrite or delete an object during its retention window. Any tampering that bypasses the API (a stolen root account on the storage host) must be *detected*, not silently accepted.
> - **R2 — Reveal nothing.** Backup data at rest must be encrypted and authenticated, and object names must not leak which files are stored.
> - **R3 — Notice the attack.** Each new backup must be scored for ransomware behaviour (mass encryption, rename storms, ransom notes) *without* flagging normal business changes.
> - **R4 — Know where "clean" is.** The system must automatically identify the most recent recovery point that pre-dates the attack — and keep doing so for every backup taken while the infection persists.
> - **R5 — Prove the restore.** Every restored byte must be cryptographically verified against what was originally backed up.
> - **R6 — Be cheap enough to run hourly.** Unchanged data must never be stored or uploaded twice, even when files are edited in the middle.
> - **R7 — Run like a cloud service.** Ship as a minimal container, deploy on Kubernetes with health probes, scheduled backup policy, and secrets management.

This is the same problem Rubrik's platform is built around ("recover before disruption strikes"). Aegis is a from-scratch, single-repository implementation of the core ideas, built to understand them properly.

---

## Architecture

```
                         ┌───────────────────────────── aegis (Go) ─────────────────────────────┐
  protected data  ─────► │  walk ─► CDC chunker ─► HMAC chunk-ID ─► dedup? ─► AES-GCM ─► worker │
  (read-only mount)      │           (FastCDC)       (keyed)         │ yes: skip     seal    pool  │──► ObjectStore
                         │                                           ▼                             │    (write-once)
                         │  per-file SHA-256 + byte entropy ─► manifest ─► ransomware detector    │      chunks/…
                         │                                       │        (vs. last CLEAN snap)    │      snapshots/…
                         │                                       └─► hash-chained, sealed manifest│
                         └─────────────────────────────────────────────────────────────────────────┘
        CLI: init · backup · list · restore --latest-clean · verify --deep        HTTP: /api/v1/*  (k8s Service + CronJob)
```

| Component | File | What it does / the interesting decision |
|---|---|---|
| Content-defined chunker | `internal/chunker` | FastCDC gear-hash with normalized chunking (2/8/64 KiB). Inserting 14 bytes at the start of a 2 MiB file still reuses **99.5%** of chunks (fixed-size blocks would reuse ~0%). Throughput ≈ **1.3 GB/s** per core. |
| Crypto | `internal/seal` | PBKDF2-SHA256 (600k iters, per-repo salt) → HMAC key separation → **keyed** chunk IDs (bucket readers can't hash-probe for known files) → AES-256-GCM with the object key as associated data (a ciphertext can't be moved to another address). |
| Object store | `internal/store` | S3-shaped interface. Local backend is WORM: atomic create-if-absent via `link(2)`, no overwrites, deletes refused during retention. |
| Backup engine | `internal/repo` | Bounded worker pool for encrypt+upload with in-flight dedup (`sync.Map`), first-error cancellation, hash-chained snapshot manifests, verified restore with path-traversal protection, deep verify. |
| Detector | `internal/detect` | Scores entropy jumps, change ratio, rename-to-bad-extension pairs and ransom notes. Compares against the **last clean** snapshot, so every backup taken during an ongoing infection stays flagged. |
| Service | `internal/server`, `cmd/aegis` | Go 1.22+ `ServeMux` routing, structured `slog` logging, single-flight backups, graceful shutdown on SIGTERM. |
| Deploy | `Dockerfile`, `deploy/k8s` | Multi-stage build → distroless non-root image. Deployment with read-only root FS, dropped capabilities, probes, Secret, PVCs, hourly CronJob policy. |
| CI | `.github/workflows/ci.yml` | gofmt, vet, `go test -race`, end-to-end demo, Docker build. |

## Results (from the test suite and demo)

| Scenario | Outcome |
|---|---|
| Incremental backup after a 1-line edit to 1 of 40 files | uploaded **8.4 KB of 415 KB** (≈2%) |
| Full attack: 80% encrypted + renamed + ransom note | score **0.95**, flagged |
| Stealthy attack: 50% encrypted **in place**, no rename, no note | score **0.62**, flagged |
| Legitimate bulk edit touching **100%** of files | score **0.20**, *not* flagged |
| Backup taken *again* while still infected | still flagged (baseline = last clean) |
| One byte flipped in one stored chunk | `verify --deep` reports it; restore refuses |
| A middle snapshot deleted from disk | hash-chain break reported |
| Chunk ciphertext moved to another chunk's address | rejected by GCM authentication |
| Wrong passphrase | rejected before any data is touched |

## Run it

```bash
go test -race ./...          # 16 tests
./scripts/demo.sh            # full attack → detect → recover story

export AEGIS_PASSPHRASE='a long passphrase'
go run ./cmd/aegis init    --repo ./repo
go run ./cmd/aegis backup  --repo ./repo ~/Documents
go run ./cmd/aegis list    --repo ./repo
go run ./cmd/aegis restore --repo ./repo --latest-clean ./restored
go run ./cmd/aegis verify  --repo ./repo --deep

docker build -t aegis:latest .
kind load docker-image aegis:latest && kubectl apply -f deploy/k8s/aegis.yaml
```

## Honest limitations (and what I'd do next)

Being clear about these is part of the design:

- **Local WORM is cooperative.** The demo shows it: an attacker with root on the storage host *can* overwrite a file. Aegis detects it (authenticated manifests + hash chain) but cannot prevent it on a local disk. **Prevention needs an enforcement point the attacker can't reach** → milestone 2.
- **Single writer per repository.** Two concurrent `backup` processes are safe for chunks (create-if-absent) but could fork the snapshot chain.
- **Detector is heuristic** and tuned on synthetic data; file types that are naturally high-entropy (zip, jpg, mp4) can't show an entropy *jump*.
- **No garbage collection** of chunks from expired snapshots yet.

## Roadmap

1. **S3 backend with Object Lock (COMPLIANCE mode)** — real immutability even against a stolen AWS root account; test with MinIO in docker-compose and LocalStack.
2. **Multi-node: distributed catalog** — Raft (hashicorp/raft) for the snapshot catalog so several agents share one repository with a single linearizable chain; leader election instead of `replicas: 1`.
3. **gRPC agent ↔ control-plane split** — lightweight agents on each host stream chunks to a central service.
4. **Mark-and-sweep GC** respecting retention locks.
5. **Prometheus metrics** (backup duration, dedup ratio, anomaly score) + alerting on suspicious snapshots.
6. **Learned detector** — per-directory baselines instead of global thresholds.

## Sources for the problem statement

- Veeam ransomware trends survey via [ComputerWeekly](https://www.computerweekly.com/news/366538492/Almost-all-ransomware-attacks-target-backups-says-Veeam) and [MSSP Alert](https://www.msspalert.com/cybersecurity-research/nearly-all-cyberattackers-target-backups-to-extort-ransoms-veeam-says-in-new-study)
- Sophos backup study via [TechRepublic](https://www.techrepublic.com/article/ransomware-attackers-target-backups/)
- Change Healthcare costs: [Paubox](https://www.paubox.com/blog/change-healthcare-ransomware-attack-projected-to-cost-2.3-billion), [IBM](https://www.ibm.com/think/news/change-healthcare-cyberattack-exceeds-1-billion-costs), [HSDL timeline](https://www.hsdl.org/c/timeline/change-healthcare-cyberattack/)
- FastCDC: Xia et al., *"FastCDC: a Fast and Efficient Content-Defined Chunking Approach for Data Deduplication"*, USENIX ATC 2016
