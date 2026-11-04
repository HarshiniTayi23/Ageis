# Aegis

**A recovery layer that survives the thing it protects you from**, whether that's a ransomware crew or a well-meaning AI agent with too much access.

[![ci](https://github.com/HarshiniTayi23/Ageis/actions/workflows/ci.yml/badge.svg)](https://github.com/HarshiniTayi23/Ageis/actions/workflows/ci.yml)

> **Status:** Milestone 1 of 3 in progress. The ransomware-vault core works and is tested. Agent undo and the distributed/AI layer are designed and on the [roadmap](docs/pm/05-roadmap.md).

---

## Why this exists

- **89%** of organisations hit by ransomware had their backups targeted (Veeam 2025). When backups fall, median recovery cost goes from **$375K to $3M** (Sophos).
- In **May 2026** an AI coding agent deleted a company's production database **and its backups in about nine seconds**, using valid credentials and no attacker (PocketOS).
- **40%** of enterprise apps are predicted to include AI agents by the end of 2026, up from under 5% in 2025 (Gartner).

Both threats destroy the same thing: the ability to recover. Full, sourced problem statement: [docs/pm/01-problem-statement.md](docs/pm/01-problem-statement.md).

## What it does

```
  scheduled backups (servers, files)  ─┐
                                       ├─► immutable vault ─► anomaly detector ─► recovery API
  AI agent action journal (DB, files) ─┘   (S3 Object Lock)     (rules → ML)      "restore clean" /
                                                                                  "undo agent X"
```

| Capability | Status |
|---|---|
| Content-defined chunking + dedup (FastCDC, ~1.3 GB/s) | ✅ M1 |
| AES-256-GCM encryption, keyed chunk addresses, PBKDF2 keys | ✅ M1 |
| Hash-chained, tamper-evident snapshot history | ✅ M1 |
| Ransomware detector (entropy jumps, rename storms, ransom notes) | ✅ M1 |
| `restore --latest-clean` with per-file SHA-256 verification | ✅ M1 |
| HTTP API, container image, Kubernetes manifests | ✅ M1 (k8s untested on a live cluster) |
| S3 backend with Object Lock (COMPLIANCE mode) | ⬜ M1 |
| AI-agent action journal + selective undo | ⬜ M2 |
| 3-node Raft catalog | ⬜ M3 |
| ML detector with evaluation set; LLM incident copilot | ⬜ M3 |

## Results so far (from the test suite)

| Scenario | Outcome |
|---|---|
| Incremental backup after a one-line edit | uploads ~2% of the data |
| 80% of files encrypted + renamed + ransom note | anomaly score 0.95, flagged |
| 50% encrypted in place, no rename, no note | score 0.62, flagged |
| Legitimate edit touching 100% of files | score 0.20, *not* flagged |
| One byte flipped in a stored chunk | `verify --deep` reports it; restore refuses |
| Snapshot deleted from disk | hash-chain break reported |

## Quick start

Needs **Go 1.24+**, **Docker**, and **make**.

```bash
git clone https://github.com/HarshiniTayi23/Ageis && cd Ageis
make setup      # creates .env, downloads modules
make check      # format, vet, lint, tests (race detector on)
make demo       # ransomware attack -> detection -> clean recovery, ~10s
make up         # local MinIO (Object Lock) + Postgres
./scripts/check-object-lock.sh   # proves even root can't delete a locked object
```

Run `make help` for every command.

## Repository layout

```
cmd/aegis/            CLI + HTTP server entrypoint
internal/
  chunker/            content-defined chunking
  seal/               key derivation and authenticated encryption
  store/              object-storage interface + local WORM backend
  repo/               backup engine: snapshot, restore, verify
  detect/             ransomware anomaly scoring
  server/             HTTP API
  journal/            (M2) AI-agent action journal
  catalog/            (M3) Raft-replicated catalog
  copilot/            (M3) LLM incident summaries
deploy/compose/       local dev stack
deploy/k8s/           Kubernetes manifests
docs/pm/              product docs: problem, personas, PRD, roadmap, AI eval plan
docs/adr/             architecture decision records
docs/learning/        learning path
```

## Built as a product, not just code

Each milestone ships a product document before the code: personas, user research, PRD, roadmap with RICE scoring, competitive analysis, and an AI evaluation plan. See [docs/pm](docs/pm/README.md). Technical trade-offs are recorded as [ADRs](docs/adr/).

## Known limitations

- The local-disk store can be overwritten by root. Aegis *detects* it but can't *prevent* it. The S3 Object Lock backend fixes that.
- Single writer per repository until the Raft catalog lands.
- The detector is heuristic and tuned on synthetic data.

## License

[MIT](LICENSE)
