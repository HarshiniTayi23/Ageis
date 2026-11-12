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

---

## 2026-11-12 · Session 1 (continued): Milestone 1 finished

### S3 Object Lock backend (PRD R6)
- **Built:** `internal/store/s3.go` plus `sigv4.go`, an S3 client on the Go standard library with my own AWS Signature V4 signer (ADR-0006).
- **Why not the AWS SDK:** the Go module mirror was unreachable here and fetching the SDK's monorepo timed out. Six S3 operations didn't justify the dependency. Trade-offs are written down: retries are ours, there's no IAM-role credential discovery, and there's a delete-marker gap.
- **How I know the signer is right:**
  - It passes **AWS's four published SigV4 examples**, all on the first run.
  - A fake S3 server re-verifies every request's signature.
  - Most convincingly, CI talks to real MinIO with it.
- **Write-once on S3:** conditional writes (`If-None-Match: *`), a COMPLIANCE retain-until date on every object, and version-specific deletes, which the service refuses.
- **Proven in CI against real MinIO, using the root credentials as the "attacker":** overwriting a snapshot gives `ErrExists`, and deleting a snapshot or chunk gives `ErrImmutable`. The ransomware is detected, the clean point restored, and the drill passes.

### `aegis drill` (PRD R7)
- Restores the latest clean snapshot to scratch space, verifies every file, deletes the scratch copy, and seals a JSON report **into the vault**, so audit evidence is write-once too. `--report file.json` writes a local copy.
- Tested that a tampered chunk makes the drill report FAIL rather than crash.

### Kubernetes proven on a real cluster (PRD R9)
- A new CI job: build the image, start a kind cluster, apply `deploy/k8s`, wait for rollout, restart the pod, trigger the CronJob once, then query `/healthz`, `/readyz` and `/api/v1/snapshots`.
- **It found two real bugs that "the manifests exist" had hidden:**
  1. The init container ran `aegis init` on every start, and the second start failed with "already initialised". That would have crash-looped on the first restart. Fixed with `init --if-missing`.
  2. `runAsNonRoot` can't be verified for an image whose user is the *name* `nonroot`. Fixed with numeric UID 65532 in the Dockerfile and the pod spec.
- Now green: rollout, restart, CronJob backup, API response.

**For review**
- ADR-0006 (own S3 client vs SDK) is the decision most likely to be challenged in an interview. Read it until you can defend both sides.
- The delete-marker gap in ADR-0006 is real. Can you explain why it *hides* data but doesn't *destroy* it?

---

## 2026-11-12 · Session 1 (continued): Milestone 2, agent undo

### PM first
- **PRD 04b:** goals, non-goals (databases, agents that bypass the tool server), requirements A1–A9, and a friction counter-metric (< 2% of normal actions delayed).
- **ADR-0007:** an event-sourced journal in the vault, served over MCP, with identity fixed at launch and undo by hash comparison.

### Built
- `internal/journal`: an append-only event log sealed in the vault. "Pending", "approved" and "undone" are **derived by replaying events**, never updated in place, because the vault is write-once.
- `internal/agentfs`: the guarded workspace.
  - Paths are confined: `..`, absolute paths and symlink escapes are refused (tested).
  - **Every write is journalled before it happens**, with the previous content stored as a deduplicated blob.
  - Deletes become *pending* until a human approves. Approval is refused if the file changed since the request.
- **Undo** walks one agent's changes newest-first and reverts only where the file still holds exactly what the agent left. Otherwise it reports a **conflict** and keeps the human's version.
- `internal/mcp`: a Model Context Protocol server over stdio, with no dependencies.
- CLI: `agent-serve`, `agent-log`, `agent-pending`, `agent-approve`, `agent-reject`, `agent-undo [--dry-run]`.
- `scripts/demo-agent.sh` (`make demo-agent`), a **PocketOS replay**, runs in CI:
  - The agent overwrites the config, tries to delete the customer database, and tries to escape to the vault.
  - The delete is held and the escape refused.
  - Undo restores the config and removes the agent's new file, but keeps the human's later edit to `notes.md` as a reported conflict.

### Competitive analysis: an important finding
- **Rubrik already ships this.** It announced Agent Rewind (undo agent actions) in Aug 2025 and Agent Identity (per-tool-call governance at an MCP gateway) in Aug 2026. Veeam and Cohesity announced similar products in 2026.
- I rewrote the positioning in the README and in `docs/pm/06` accordingly: Aegis is a from-scratch, inspectable implementation of the problems Rubrik's newest products solve, **not** a claim of a new idea.
- Possible design differences worth discussing: conflict-aware undo, human approval as an event, and the journal living in the same write-once vault as backups.

**For review**
- **Read `docs/pm/06` before any Rubrik interview.** Not knowing that Rubrik ships Agent Rewind while presenting "agent undo" would be the single worst possible moment in the interview.
- Do you agree that only deletes should be held, and overwrites only recorded? That's hypothesis H3. Holding overwrites too would be safer but slower.

---

## 2026-11-12 · Session 1 (continued): M3 begins, measuring the detector honestly

### Why this came before Raft
The roadmap put Raft first in M3 for strategic reasons. But the PRD's recall target (≥ 95%) had never been measured against anything except scenarios I'd chosen myself. Shipping an "AI" feature without an evaluation is the classic AI-PM mistake, so the evaluation came first.

### What I built
- `cmd/aegis-eval` (`make eval`): 440 labelled snapshots from 22 scenarios × 20 seeds, using real files and real entropy, with hard cases on both sides.
- **The held-out set was written before any tuning** and never tuned against.

### What it found
- **v1 recall was 38%** (33% held-out), not the near-100% the unit tests implied.
- It missed wipers, renames to unknown extensions, base64-wrapped ciphertext, and encryption of already-compressed files.

### v2 detector
- Pairs renamed and moved files with their originals.
- Sniffs file formats, so a broken JPEG or PDF signature counts as hostile even with no entropy change.
- Treats mass deletion as a signal.
- Uses a lower entropy-jump threshold for text.

### Where it got to
- **The first v2 attempt hit 75% recall but 9% false alarms**, breaking the PRD's 2% counter-metric: a legitimate password-protected export looked like an attack.
- I fixed it with domain reasoning (legitimate tools don't mass-corrupt *binary* formats), not by fitting the data.
- **Final: 87% recall on tuning, 68% on held-out, 0% false alarms on both.**
- Remaining misses: slow encryption, which needs history (the M3 statistical detector), and XOR on the held-out set, deliberately not fixed.

### Guarding the result
- CI now **gates** on detection quality: recall ≥ 85% and false alarms ≤ 2%. Held-out recall is reported but not gated.
- New detector unit tests cover the wiper, rename pairing, binary-format breakage, and moves that must *not* be flagged.

### Process notes, for honesty
- Before tuning, I fixed a realism bug in my own generator: the fake JPEGs lacked the JPEG file signature. It's a data fix, not tuning, and the v1 numbers were re-recorded after it. Both runs gave the same headline (38% / 33%).
- The scenarios were written by the detector's author. Real-world recall is unknown. The eval docs say so up front.

**For review**
- This is your best AI-PM interview story: *"My tests said ~100%. My evaluation said 38%. My first fix broke the false-alarm budget. Here's how I chose between recall and precision, and why the held-out number is the one I quote."*
- The PRD target of 95% recall is **not met** (68% held-out). Do you lower the target, or keep it and plan the history-based detector? I kept it. A PM shouldn't move the goalposts to declare victory.
