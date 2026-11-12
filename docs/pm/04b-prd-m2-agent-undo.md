# 04b · PRD: Milestone 2, agent undo

**Status:** Draft v0.1 · **Owner:** Sri Harshini Tayi · **Last updated:** 2026-11-12 · **Persona:** Arjun (primary) from [02](02-personas.md)

## 1. Summary
AI agents now get write access to company files and data. In May 2026 one deleted a production database *and its backups* in nine seconds, with valid credentials and no attacker. Arjun wants to keep the speed of agents without betting the company on them. M2 gives agents their file tools *through* Aegis: every change is journalled in the immutable vault under the agent's identity, deletions wait for a human, and one agent's work can be undone without losing anyone else's.

## 2. Goals
- **G1 Accountability:** every change an agent makes is recorded with who, what, when, and the before and after content. The agent cannot erase its own record.
- **G2 Precise undo:** `undo --agent X` reverts X's changes and nothing else.
- **G3 Do no harm while undoing:** if a human changed a file after the agent, undo leaves the human's version and reports a conflict.
- **G4 Low friction:** normal agent work (read, create, edit) is never blocked.

## 3. Non-goals (this milestone)
- **Databases.** Postgres change capture is M2.5.
- **Judging whether an edit is *good*.** We record and reverse; we don't review code.
- **Agents that don't use our tools.** An agent with its own shell and credentials bypasses Aegis. The product answer is: don't give agents raw credentials. That's a deployment rule, written in the docs, not something code can enforce.

## 4. Requirements

| ID | Requirement | Priority |
|---|---|---|
| A1 | File tools for agents over MCP: read, list, write, delete, confined to one workspace directory (no `..`, absolute paths or symlink escapes) | Must |
| A2 | The agent's identity is fixed when the tool server starts; the agent cannot choose or change it | Must |
| A3 | Every write and delete is journalled in the vault *before* it happens, including the previous content | Must |
| A4 | Deletes are held as **pending** until a human approves them; the agent is told so in plain words | Must |
| A5 | `aegis agent log` shows the timeline, filterable by agent | Must |
| A6 | `aegis agent undo --agent X --dry-run` shows exactly what would change; without `--dry-run` it applies it | Must |
| A7 | Undo never overwrites a file whose current content differs from what the agent left; it reports a conflict instead | Must |
| A8 | Undo actions are themselves journalled | Should |
| A9 | Rate guard: more than N writes per minute from one agent are held | Could (M2.5) |

## 5. Success metrics

| Metric | Target |
|---|---|
| Agent changes recoverable by undo | 100% of journalled writes and deletes |
| Human changes lost by an undo | 0 (conflicts reported instead) |
| Non-destructive agent actions delayed | < 2% *(counter-metric: friction)* |
| Time from "agent misbehaved" to "undone" | < 2 minutes in the demo |

## 6. Key flow: replaying PocketOS
1. An agent working in the workspace edits a few files normally. All are journalled; none are blocked.
2. A human edits `notes.md` by hand.
3. The agent "goes rogue": it overwrites `config.yaml` with garbage, edits `notes.md` after the human, and tries to delete `customers.db`.
4. The delete is **held**: "pending human approval (id …)". `customers.db` is untouched.
5. Arjun runs `aegis agent log --agent build-bot`, then `aegis agent undo --agent build-bot --dry-run`, then applies it.
6. `config.yaml` is restored. `notes.md` is reported as a **conflict**, because the agent's last write came after the human's. Arjun decides by hand, and the human's text is still recoverable from the journal.

## 7. Risks

| Risk | Mitigation |
|---|---|
| Arjun's team rejects approvals as friction (H3 false) | Only deletes are held; measure in user interviews |
| Journal grows with every write | Content-addressed and deduplicated blobs; retention policy is roadmap work |
| Agents get raw credentials anyway | Deployment guidance; the M2.5 Postgres capture works at the database layer, so even raw-credential agents are recorded |
