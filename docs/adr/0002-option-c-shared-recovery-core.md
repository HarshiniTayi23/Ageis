# ADR-0002: One shared recovery core for ransomware and AI-agent damage

**Status:** Accepted · **Date:** 2026-11-04

## Context
Two threats destroy the same thing, the ability to recover: ransomware crews (89% target backups) and over-privileged AI agents (e.g. PocketOS, May 2026). See [problem statement](../pm/01-problem-statement.md).

## Decision
Build one vault, catalog and recovery engine, with two ingest paths:
1. **Scheduled backups** of files and servers (package `repo`).
2. **An agent action journal** recording what each AI agent changed (package `journal`).

Both feed the same immutable store, anomaly detector and recovery API.

## Options considered
| Option | Pros | Cons |
|---|---|---|
| A: ransomware vault only | Smallest, well understood | Common project; AI only in the detector |
| B: agent undo only | Novel, current | Harder to frame; less public data |
| **C: shared core (chosen)** | Mirrors Rubrik's data + identity + AI platform; each slice is demoable | Biggest scope; risk of scope creep |

## Consequences
- Milestones are vertical slices (M1 ransomware, M2 agents, M3 distributed + AI) so the project is presentable after each one.
- The store, crypto and catalog must stay generic enough for both ingest paths. Neither may depend on the other.
- Scope creep is managed through the roadmap's RICE scoring and its explicit cut list.
