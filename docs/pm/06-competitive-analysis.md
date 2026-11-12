# 06 · Competitive analysis

**Status:** Draft v0.1 · **Last updated:** 2026-11-12

## The headline finding
**The "undo for AI agents" idea is not new. Rubrik itself ships it.** Rubrik announced **Agent Rewind** in August 2025 and **Agent Identity**, governing agents one tool call at a time through an **MCP gateway**, in August 2026. Veeam and Cohesity followed with their own versions in 2026.

That changes how to present Aegis. It's not "a startup idea Rubrik missed". It's **a from-scratch, inspectable implementation of the problems Rubrik's newest products solve**, built to understand them deeply enough to work on them. For an interview at Rubrik, that's a stronger story: "I built a small version of your Agent Rewind and MCP gateway, and here's what I learned about the hard parts."

## Landscape

| | Rubrik | Veeam | Cohesity | Aegis (this project) |
|---|---|---|---|---|
| **Agent undo product** | Agent Rewind (announced Aug 2025) | Agent Commander (announced Feb 2026; "future release") | Recoverability service with ServiceNow and Datadog (announced Mar 2026, due by end of 2026) | `aegis agent-undo` (M2) |
| **How actions are attributed** | Maps agent behaviour, tool use and impact; links each action to a prompt, plan or tool (Predibase technology) | "Data Command Graph" linking identities, data and agents | Observability partners flag anomalies | Identity fixed at tool-server launch; every write journalled with that identity |
| **Granularity of undo** | Pick a recovery point (latest good or earlier) via a ticketed workflow | "Precise, context-aware rollbacks… without reverting entire systems" | Point-in-time recovery of agents, data, vector stores, agent memory | Per-agent, per-file, newest first; **refuses** to overwrite later human edits |
| **Prevention before the action** | Agent Identity: behavioural analysis, a policy check and a scoped short-lived token **per tool call**, at an MCP gateway (Aug 2026) | Real-time controls across data, identities, agents | Not described | Path confinement; **deletes held for human approval** |
| **Scope** | Files, databases, configurations, repositories; Agentforce, Copilot Studio, Bedrock Agents, custom agents | Production and backup data, identities, models, agents | Files, DBs, object storage, SaaS, vector stores, agent memory | Files only (Postgres in M2.5) |
| **Immutable storage underneath** | Yes (core platform) | Yes | Yes | Yes: S3 Object Lock COMPLIANCE; the journal lives in the same vault |

*Sources: vendor announcements and trade press, linked below. Capabilities are as announced; availability dates vary and several were "upcoming" when announced.*

## What Aegis does differently, honestly

These are design choices worth discussing, not claims of superiority over products with real customers.

1. **Conflict-aware undo.** Undo compares hashes and leaves a file alone if anyone changed it after the agent. The vendor materials don't describe how they handle a human edit made after an agent's edit. That's a good interview question to ask *them*.
2. **Human approval as a first-class event.** Deletes become pending events in an event-sourced log. Rubrik's Agent Identity materials describe automated policy blocking but no human approval step.
3. **The journal sits in the same write-once vault as the backups**, so the agent's audit trail has the same tamper resistance as the data. The PocketOS lesson was that the agent could reach the backups.

## Where Aegis is clearly weaker
- Files only; no SaaS, databases, vector stores or agent memory.
- Identity is a launch flag, not federated with Okta or Entra ID, and there are no short-lived per-call tokens.
- No behavioural analysis of agent intent; prevention is rule-based.
- Single node, no UI, no customers.

## If I were Rubrik's PM

| Option | Assessment |
|---|---|
| **Build** | They did: Agent Rewind and Agent Identity. |
| **Next gap I'd look at** | Undo *with conflict handling* across humans and agents, since real teams interleave edits. Also approval UX that doesn't kill agent speed: measure the delayed-action rate, our M2 counter-metric. |
| **Biggest risk** | Agents that bypass the gateway with raw credentials. This is why Rubrik pairs the gateway (Agent Identity) with storage-level recovery (Rewind). Aegis makes the same split: tool server plus vault. |

## Questions this raises for the problem statement
- The problem is real and validated *by the market*. Three major vendors shipped or announced products within a year.
- The open question is no longer "is this a problem?" but "which part is still unsolved?" That's a sharper research question for user interviews: *"If you use Rubrik or Veeam's agent undo, what does it get wrong?"*

## Sources
- [Business Wire – Rubrik Unveils Agent Rewind (Aug 2025)](https://www.businesswire.com/news/home/20250812418116/en/Rubrik-Unveils-Agent-Rewind-For-When-AI-Agents-Go-Awry)
- [Computerworld – Rubrik's way to reverse AI agent mistakes](https://www.computerworld.com/article/4037957/ai-agents-make-mistakes-rubrik-has-figured-out-a-way-to-reverse-them.html)
- [SiliconANGLE – Rubrik Agent Identity (Aug 2026)](https://siliconangle.com/2026/08/04/rubrik-unveils-agent-identity-govern-ai-agents-one-tool-call-time/)
- [Rubrik – Adds MCP support](https://www.rubrik.com/company/newsroom/press-releases/26/rubrik-adds-new-mcp-support-to-expand-access-to-agentic-cyber-resilience)
- [Help Net Security – Veeam Agent Commander (Feb 2026)](https://www.helpnetsecurity.com/2026/02/24/veeam-agent-commander/)
- [The Register – Cohesity agentic AI rollback (Mar 2026)](https://www.theregister.com/2026/03/10/agentic_ai_rollback_recovery_cohesity/)
