# 05 · Roadmap

**Status:** Draft v0.1 · Milestones are vertical slices: each one ends with something demoable, so the project is presentable at any point.

## Milestones

### M1 · Ransomware vault *(foundation largely built)*
**Outcome:** an admin can recover the last clean copy of their data after a simulated ransomware attack, from storage the attacker cannot delete.
- ✅ Content-defined chunking + dedup, encryption, hash-chained snapshots
- ✅ Rule-based ransomware detector, `restore --latest-clean`, deep verify
- ⬜ **S3 backend on MinIO with Object Lock (COMPLIANCE)**: closes the "root can overwrite local files" gap
- ⬜ PRD and personas written
- ⬜ Demo video #1

### M2 · Agent undo
**Outcome:** an AI platform owner can see exactly what one agent changed and roll back only that, without losing human work.
- ⬜ Agent action journal: capture every agent write (MCP proxy for tools, Postgres change-data-capture for the database)
- ⬜ Guardrails: destructive actions (DROP, DELETE without WHERE, bucket deletion) are delayed or need approval
- ⬜ Selective rollback by agent identity
- ⬜ Replay of the PocketOS scenario as a demo
- ⬜ Competitive analysis

### M3 · Distributed and AI depth
**Outcome:** the system survives a node failure, and an incident summary tells the admin what happened in plain language, with every claim checked against real data.
- ⬜ 3-node catalog with Raft
- ⬜ Kubernetes deployment on kind, Prometheus metrics
- ⬜ Statistical/ML detector with a labelled evaluation set
- ⬜ LLM incident copilot, measured against the AI evaluation plan

## How we prioritise: RICE

For any feature competing for time: **score = Reach × Impact × Confidence ÷ Effort**.

| Feature | Reach (1–10) | Impact (0.25–3) | Confidence (%) | Effort (days) | Score |
|---|---|---|---|---|---|
| S3 Object Lock backend | | | | | |
| Agent action journal | | | | | |
| Raft catalog | | | | | |
| LLM incident copilot | | | | | |

_Exercise: fill this in, then check whether the order above still makes sense. If it doesn't, change the roadmap and write down why._

## Explicitly cut (for now)
- Data-exfiltration detection (out of scope, see problem statement)
- Windows support
- A web UI (CLI + API is enough to demo)
