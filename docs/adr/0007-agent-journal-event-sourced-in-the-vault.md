# ADR-0007: Agent journal as an event log in the vault, served to agents over MCP

**Status:** Accepted · **Date:** 2026-11-12

## Context
M2 must record every agent change, hold deletes for approval, and undo one agent's work selectively ([PRD 04b](../pm/04b-prd-m2-agent-undo.md)). Two constraints shape the design:
- The vault is **write-once**. Nothing can be updated in place, including the status of a pending approval.
- In the PocketOS incident the agent destroyed backups *because it could reach them*. The agent must never hold vault credentials.

## Decision
1. **Event sourcing.** The journal is an append-only list of sealed events in the vault (`journal/<time>-<rand>`): `write`, `delete-requested`, `delete-approved`, `undo`. Current state, such as "is delete X still pending?", is *derived* by replaying events, never stored. This fits write-once storage exactly, and the history can't be rewritten.
2. **Before-images.** Before any change, the previous file content is stored as a content-addressed, sealed blob (`journal-blobs/<keyed hash>`), so identical content is stored once.
3. **Agent access through an MCP server.** Agents call `read_file`, `list_directory`, `write_file` and `delete_file` through Aegis, using the Model Context Protocol over stdio, a standard most agent tools speak. The server process holds the vault credentials; the agent only sees tool results.
4. **Identity at launch.** `aegis agent serve --agent NAME` fixes the identity for the process lifetime. No tool argument can change it.
5. **Undo by hash comparison.** Walk the agent's events newest-first. For each one, revert only if the file's current SHA-256 equals what the agent left behind; otherwise report a conflict and skip.

## Options considered
| Option | Pros | Cons |
|---|---|---|
| **Event log in the vault (chosen)** | Immutable audit trail; agent can't erase its tracks; one storage system | Reads replay all events (fine at demo scale; needs an index later) |
| Mutable SQLite journal | Simple queries and status updates | Agent or attacker on the host can edit history; a second storage system |
| Filesystem snapshots (btrfs/ZFS) | Captures everything | Can't attribute changes to an agent; rollback is all-or-nothing, the exact problem we're solving |
| FUSE filesystem interception | Works with any agent, no tool changes | Platform-specific, complex, hard to test in CI |

## Consequences
- Undo cannot lose human work, by construction: a mismatch is always a conflict.
- Event sourcing is a good interview topic: same idea as Kafka-backed systems, bank ledgers and Git.
- Not covered: agents that bypass the tool server. That's a deployment rule in the docs, with database-layer capture in M2.5.
