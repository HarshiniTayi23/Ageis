# Aegis

**A recovery layer that survives the thing it protects you from**, whether that's a ransomware crew or a well-meaning AI agent with too much access.

[![ci](https://github.com/HarshiniTayi23/Ageis/actions/workflows/ci.yml/badge.svg)](https://github.com/HarshiniTayi23/Ageis/actions/workflows/ci.yml)

> **Status:** Milestones 1 (ransomware vault) and 2 (agent undo) are built and tested in CI, including against real S3 Object Lock and a live Kubernetes cluster. Milestone 3 (distributed catalog, ML detector, LLM copilot) is next. See the [roadmap](docs/pm/05-roadmap.md) and the [build log](docs/BUILD_LOG.md).

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
  AI agent action journal (files)     ─┘   (S3 Object Lock)     (rules → ML)      "restore clean" /
                                                                                  "undo agent X"
```

| Capability | Status |
|---|---|
| Content-defined chunking + dedup (FastCDC, ~1.3 GB/s) | ✅ M1 |
| AES-256-GCM encryption, keyed chunk addresses, PBKDF2 keys | ✅ M1 |
| Hash-chained, tamper-evident snapshot history | ✅ M1 |
| Ransomware detector: entropy, format breakage, rename/move pairing, mass deletion, notes | ✅ v2: 87% recall (68% on held-out), 0% false alarms on 440 labelled snapshots ([eval](docs/eval/README.md)) |
| `restore --latest-clean` with per-file SHA-256 verification | ✅ M1 |
| HTTP API, container image, Kubernetes manifests (deployed to kind in CI) | ✅ M1 |
| S3 backend with Object Lock (COMPLIANCE mode); stdlib SigV4 client | ✅ M1 |
| `aegis drill`: verified test restore with a sealed audit report | ✅ M1 |
| MCP tool server for AI agents: every write journalled in the vault | ✅ M2 |
| Deletes held for human approval; undo one agent, keep human edits | ✅ M2 |
| Postgres change capture for agents | ⬜ M2.5 |
| 3-node Raft catalog | ⬜ M3 |
| Detector evaluation harness + CI quality gate | ✅ M3 |
| History-based detector (slow encryption); LLM incident copilot | ⬜ M3 |

## Results so far (from the test suite)

| Scenario | Outcome |
|---|---|
| Incremental backup after a one-line edit | uploads ~2% of the data |
| 80% of files encrypted + renamed + ransom note | anomaly score 0.95, flagged |
| 50% encrypted in place, no rename, no note | score 0.62, flagged |
| Legitimate edit touching 100% of files | score 0.20, *not* flagged |
| One byte flipped in a stored chunk | `verify --deep` reports it; restore refuses |
| Snapshot deleted from disk | hash-chain break reported |
| Root user tries to overwrite or delete a backup on S3 Object Lock | refused by the storage service (CI, real MinIO) |
| AI agent overwrites config, tries to delete the database | delete held; undo restores config, keeps the human's later edit |

## Quick start

Needs **Go 1.24+**, **Docker**, and **make**.

```bash
git clone https://github.com/HarshiniTayi23/Ageis && cd Ageis
make setup      # creates .env, downloads modules
make check      # format, vet, lint, tests (race detector on)
make demo       # ransomware attack -> detection -> clean recovery, ~10s
make demo-agent # rogue AI agent -> contained -> undone (PocketOS replay)
make up         # local MinIO (Object Lock) + Postgres
./scripts/check-object-lock.sh   # proves even root can't delete a locked object
make integration                 # full backup/attack/recovery against MinIO
```

Run `make help` for every command.

## Repository layout

```
cmd/aegis/            CLI + HTTP server entrypoint
internal/
  chunker/            content-defined chunking
  seal/               key derivation and authenticated encryption
  store/              object-storage interface, local WORM, S3 (SigV4) backends
  repo/               backup engine: snapshot, restore, verify, drill
  detect/             ransomware anomaly scoring
  server/             HTTP API
  journal/            AI-agent action journal (event-sourced, in the vault)
  agentfs/            guarded workspace: confinement, held deletes, undo
  mcp/                Model Context Protocol server (stdio)
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

**On originality:** the [competitive analysis](docs/pm/06-competitive-analysis.md) found that Rubrik already ships agent undo (Agent Rewind, 2025) and per-tool-call agent governance at an MCP gateway (Agent Identity, 2026), and Veeam and Cohesity have announced similar products. Aegis doesn't claim the idea. It's a from-scratch, fully inspectable implementation of these problems, built to understand where the hard parts are, such as undoing an agent without losing a human's later edit.

## Known limitations

- The local-disk store can be overwritten by root. Aegis *detects* it but can't *prevent* it. Use the S3 backend (`--store s3`) for real immutability.
- On S3, a writer can add a delete marker that *hides* (not destroys) the latest version; recovery by version listing is roadmap work (ADR-0006).
- Agent undo only covers agents that act through Aegis's tool server. An agent given raw credentials bypasses it (ADR-0007).
- Single writer per repository until the Raft catalog lands.
- The detector is heuristic and tuned on synthetic data written by its author: 87% recall on tuning scenarios, 68% on held-out ones. It misses slow encryption spread over many backups.

## License

[MIT](LICENSE)
