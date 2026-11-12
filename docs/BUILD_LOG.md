# Build log

A running record of what was built, what was decided, what broke, and what's still open. Newest entries at the bottom. Each entry ends with **For review**: decisions you may want to overturn.

---

## 2026-11-12 · Session 1: CI repair and the start of autonomous build

### CI was red: MinIO's images vanished (twice)
- **What happened:** the first CI run failed with `pull access denied for minio/minio`. Docker Hub stopped serving MinIO's images around 12 Sept 2026, after the upstream project was archived in April. I moved to Quay (ADR-0004); the next run failed with `401 UNAUTHORIZED`, because Quay had withdrawn them too.
- **Fix:** Chainguard's maintained MinIO build for the server, and the AWS CLI for bucket setup and the lock check, instead of MinIO's `mc` (ADR-0005).
- **Result:** all three CI jobs green. The log shows the root user's delete refused: *"Object is WORM protected and cannot be overwritten."*
- **Lesson:** pinning a version protects against *changes*, not *disappearance*.
- **Process slip I corrected:** I briefly edited the accepted ADR-0003, against our own ADR-0001 rule. I reverted it and added only a "superseded by" pointer.

### PM: personas v0.2, RICE-scored roadmap, M1 PRD
- **Personas:** I answered the three open questions provisionally so building could continue:
  - Ravi is primary for M1.
  - Rubrik's real customers are bigger, which justifies M3.
  - The riskiest assumptions are H1 (finding a clean point is slow) and H3 (agent delays are acceptable).
- **RICE scoring changed the roadmap:**
  - Added **`aegis drill`** to M1: Meera's need for proof before an attack, cheap to build.
  - **Agent capture starts with files, not Postgres** (half the effort, same user value).
  - **Raft stays in M3 despite the lowest score.** RICE doesn't capture strategic value; the override is written down.
- **PRD for M1:** ten requirements (six done, four open), metrics with a counter-metric (false alarms ≤ 2%), user flows, risks, and an M2 sketch.

**For review**
- Do you agree Ravi, not Meera, is the M1 primary?
- Is "Raft stays despite RICE" a call you can defend in an interview? I think yes, *because* it's written down.
- The RICE numbers are my estimates. Re-scoring them yourself is the fastest way to own them.
