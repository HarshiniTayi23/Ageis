# 04 · PRD: Milestone 1, ransomware vault

**Status:** Draft v0.1 · **Owner:** Sri Harshini Tayi · **Last updated:** 2026-11-12 · **Personas:** Ravi (primary), Meera (secondary), from [02](02-personas.md)

A PRD answers *what* and *why*. The *how* lives in [ADRs](../adr/) and code.

## 1. Summary
Ransomware crews target backups in 89% of attacks, and when backups fall, recovery costs roughly eight times more. Ravi, an infrastructure lead in a small IT team, needs to find the last clean copy of his data and restore it at 3 a.m. Meera, his CISO, needs proof beforehand that this will work. M1 delivers a backup vault that an attacker holding admin credentials cannot delete, that flags infected backups, and that restores and proves the last clean point.

## 2. Goals
- **G1 Survive:** no credential the attacker can steal, including storage root, can delete or overwrite a backup inside its retention window.
- **G2 Know where clean is:** Ravi gets the last clean recovery point from one command, with the reason any later points were rejected.
- **G3 Prove it:** every restore is verified, and Meera can schedule test restores with a report she can hand to auditors.
- **G4 Be cheap enough to run hourly:** unchanged data is never stored or uploaded twice.

## 3. Non-goals
- **Preventing the break-in.** That's endpoint and network security's job.
- **Data theft.** Backups can't un-steal data, and theft-only extortion stays out of scope.
- **AI agents.** That's M2.
- **A web UI.** CLI and HTTP API only.
- **Windows.** Linux and macOS only.
- **Multi-node.** That's M3.

## 4. Requirements

| ID | Requirement | Persona | Priority | Status |
|---|---|---|---|---|
| R1 | Backups are encrypted and authenticated at rest; object names reveal nothing about file contents | Meera | Must | ✅ |
| R2 | Unchanged data is deduplicated, including after mid-file edits | Ravi | Must | ✅ |
| R3 | Each new snapshot gets an anomaly score and human-readable reasons | Ravi | Must | ✅ |
| R4 | `restore --latest-clean` picks the newest snapshot not flagged, and keeps rejecting snapshots taken during an ongoing infection | Ravi | Must | ✅ |
| R5 | Every restored file is verified against its original SHA-256, and a mismatch aborts the restore | Ravi, Meera | Must | ✅ |
| R6 | Backups can be stored in S3-compatible storage with COMPLIANCE Object Lock; overwrite and delete attempts fail even for root | Meera | Must | ⬜ |
| R7 | `aegis drill` restores the latest clean point to a scratch location, verifies it, and writes a JSON report (snapshot, file count, bytes, duration, pass/fail) | Meera | Should | ⬜ |
| R8 | Any tampering with stored data is detected by `verify` | Meera | Must | ✅ |
| R9 | Runs as a container on Kubernetes with health probes and an hourly schedule | Ravi | Should | ⬜ (manifests exist, unproven) |
| R10 | A wrong passphrase is rejected before any data is read | Ravi | Must | ✅ |

## 5. Success metrics

| Metric | Baseline | Target | How we measure it |
|---|---|---|---|
| Time from "attack noticed" to "clean point identified" | Unknown. H1 says hours to days; to be measured in user research | < 1 minute | Demo script timing |
| Attack detection rate (recall) | v1: 38% / 33% held-out (measured) | ≥ 95% of simulated attacks | `make eval`; v2 is at 87% tuning, 68% held-out (see docs/eval) |
| False alarms on normal business changes | v1: 0% | ≤ 2% of clean snapshots | `make eval`, gated in CI; v2 is at 0% |
| Restored files verified | n/a | 100% | Built into restore |
| Upload size for an hourly backup with small changes | Full copy = 100% | < 5% of total size | Backup stats |
| Successful deletes of locked objects | n/a | 0 | CI Object Lock check |

**Counter-metric:** false alarms. If Ravi learns to ignore alerts, detection is worthless whatever its recall. A change that raises recall but pushes false alarms above 2% does not ship.

## 6. Key user flows

**Ravi at 3 a.m.**
1. The hourly backup runs and prints `RANSOMWARE INDICATORS – anomaly score 0.95` with reasons.
2. `aegis list` shows the timeline: clean, clean, SUSPICIOUS.
3. `aegis restore --latest-clean /restore` names the recovery point and restores it, with every file verified.

**Meera, monthly**
1. A scheduled `aegis drill` restores the latest clean point to scratch space.
2. It writes `drill-report.json`: pass/fail, snapshot ID and time, files, bytes, duration.
3. She attaches the report to the audit evidence.

## 7. Risks and open questions

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| H1 is false: clean points are already easy to find | Medium | High: weakens the pitch | User interviews; pitch falls back to immutability and proof |
| Detector tuned only on synthetic data | High | Medium | Say so openly; labelled evaluation set in M3 |
| Local MinIO image disappears again | Medium (it happened twice) | Low: dev only | ADR-0005 fallback list; S3 client uses only the standard S3 API |
| An attacker deletes the whole bucket via the cloud account | Low | High | COMPLIANCE mode blocks bucket deletion while objects are locked; recommend a separate cloud account for the vault |

**Open question:** should retention be set per object (flexible) or only as the bucket default (simpler, harder to misuse)? Current answer: bucket default, plus a per-object retain-until on each write as defence in depth. See ADR-0006.

## 8. Launch plan
M1 is "launched" when: CI is green, including a live Object Lock check and a kind deployment; the README demo shows the attack, detection and recovery; and a two-minute demo video exists.

---

# Appendix: M2 sketch (agent undo)

To be expanded into its own PRD before M2 code.

- **Persona:** Arjun.
- **Job:** undo exactly what one agent did.
- **Must:**
  - Every agent file operation is journalled with agent ID, time, and the before and after state, stored in the vault.
  - Destructive operations (delete, overwrite of a large file, bulk changes) are held for N seconds or until approved.
  - `aegis undo --agent X` reverts only X's changes and **refuses** to overwrite a file a human changed afterwards, reporting the conflict instead.
- **Counter-metric:** guardrail friction. Fewer than 1 in 50 non-destructive agent actions should be delayed.
