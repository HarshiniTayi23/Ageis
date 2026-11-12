# 05 · Roadmap

**Status:** Draft v0.2 (RICE-scored 2026-11-12) · Milestones are vertical slices: each one ends with something demoable, so the project is presentable at any point.

## Milestones

### M1 · Ransomware vault
**Outcome:** Ravi can recover the last clean copy of his data after a ransomware attack, from storage the attacker cannot delete, and Meera can prove beforehand that recovery works.
- ✅ Content-defined chunking + dedup, encryption, hash-chained snapshots
- ✅ Rule-based ransomware detector, `restore --latest-clean`, deep verify
- ✅ **S3 backend with Object Lock (COMPLIANCE)**: closes the "root can overwrite local files" gap
- ✅ **`aegis drill`**: scheduled test restore with an audit report *(new in v0.2, came out of RICE scoring)*
- ✅ Kubernetes manifests proven on a real cluster (kind, in CI)
- ✅ Personas (v0.2) · ✅ PRD · ⬜ Demo video #1 *(needs you)*

### M2 · Agent undo
**Outcome:** Arjun can see exactly what one agent changed and roll back only that, without losing human work.
- ✅ **Guarded file-tool server for agents (MCP)**: agents read and write files through Aegis, and every change is journalled with the agent's identity
- ✅ Guardrails: deletes are held for human approval *(bulk-overwrite rate guard: M2.5)*
- ✅ Selective undo by agent, with conflict detection when a human edited the same file later
- ✅ Replay of the PocketOS scenario as a demo (`make demo-agent`, runs in CI)
- ⬜ Postgres change capture *(moved to M2.5, see "What changed")*
- ✅ Competitive analysis (finding: Rubrik already ships Agent Rewind; see 06)

### M3 · Distributed and AI depth
**Outcome:** the system survives a node failure, and an incident summary tells Ravi what happened in plain language, with every claim checked against real data.
- ⬜ 3-node catalog with Raft
- ⬜ Prometheus metrics
- ✅ Labelled evaluation set (440 snapshots, held-out split) + CI quality gate; rule-based v2 detector (recall 38% → 87% tuning / 68% held-out, 0% false alarms)
- ⬜ Statistical detector using snapshot history (targets slow encryption, the main remaining miss)
- ⬜ LLM incident copilot, measured against the AI evaluation plan

## RICE scores

**Score = Reach × Impact × Confidence ÷ Effort**
- **Reach:** share of target users who benefit, on a 1–10 scale.
- **Impact:** 3 massive, 2 high, 1 medium, 0.5 low.
- **Effort:** engineering days.

| Feature | Reach | Impact | Confidence | Effort (days) | Score | Milestone |
|---|---|---|---|---|---|---|
| S3 Object Lock backend | 9 | 3 | 90% | 2 | **12.2** | M1 |
| `aegis drill` (verified test restore + report) | 7 | 2 | 70% | 1.5 | **6.5** | M1 |
| k8s proven on kind | 5 | 1 | 90% | 1 | **4.5** | M1 |
| Agent journal + undo (file tools) | 6 | 3 | 60% | 5 | **2.2** | M2 |
| LLM incident copilot | 7 | 1 | 50% | 3 | **1.2** | M3 |
| Postgres change capture for agents | 5 | 2 | 50% | 5 | **1.0** | M2.5 |
| Raft catalog | 4 | 1 | 70% | 5 | **0.6** | M3 |

## What changed in v0.2, and why

1. **`aegis drill` was added to M1.** Writing Meera's persona surfaced "proof before the attack" as her top need. It's cheap because it reuses restore and verify. RICE puts it second overall.
2. **Agent capture starts with files, not Postgres.** Same user value (undo one agent), less than half the effort, and no database driver dependency. Postgres moves to M2.5.
3. **Raft scores last but stays in M3. RICE doesn't capture strategic value.** For *this* project, demonstrating distributed-systems depth is a goal in itself; it's the first line of the job description. A real PM would make the same call for a strategic bet and write down that the score was overridden, as done here.
4. **The copilot outscores Raft but stays after it.** It depends on a trustworthy detector and the evaluation plan existing first.

## Explicitly cut (for now)
- Data-exfiltration detection (out of scope, see problem statement)
- Windows support
- A web UI (CLI + API is enough to demo)
