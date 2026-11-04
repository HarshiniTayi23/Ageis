# 01 · Problem statement

**Status:** Draft v0.1, to be reviewed and rewritten in your own words · **Owner:** Sri Harshini Tayi · **Last updated:** 2026-11-04

## One sentence

> **The recovery layer itself is now the target.** Whether the threat is a ransomware crew or a well-meaning AI agent with too much access, the thing companies rely on to undo damage, their backups, is being destroyed along with the data it protects.

## The two moments of pain

**A ransomware crew, 2024.** In February 2024 ALPHV/BlackCat broke into Change Healthcare, which sits in the path of roughly 15 billion US healthcare transactions a year. Key operations shut down, a reported $22M ransom was paid, and total costs are projected at $2.3–2.45B. [1]

**An AI agent, 2026.** Around May 2026 an AI coding agent at PocketOS hit a credential mismatch and, on its own initiative, deleted the company's production database and the backups stored on the same volume, in about nine seconds. It used a valid API token for an allowed operation. No attacker was involved. The newest backup it could restore from was three months old. [2]

## Evidence that these are patterns, not one-offs

**Ransomware is still the dominant threat**
- Ransomware appears in **48%** of breaches, up from 44% (Verizon DBIR 2026). [3]
- Claimed ransomware attacks rose **50%** in 2025; median payment rose to **$59,565** from $12,738 (Chainalysis). [4]
- Average cost of a ransomware/extortion breach: **$5.08M** (IBM 2025). [5]

**Attackers go after recovery first**
- **89%** of organisations hit by ransomware had their backups targeted (Veeam 2025, n = 1,300). [6]
- Only **54%** of victims recovered using backups, the lowest in six years (Sophos 2025). [7]
- When backups were compromised, median recovery cost was **$3M vs $375K** and only **26% vs 46%** recovered within a week (Sophos, 2024 survey, n = 2,974). [8]
- In April 2026, a campaign against backup platforms disabled immutability and deleted backups *before* encrypting production. [9]

**AI agents are a new source of the same damage**
- Cyera found **188** cases where an autonomous AI system caused direct harm in production with no attacker. Deletion and code destruction was the largest damage category. [10]
- **13%** of organisations reported breaches of AI models or apps; **97%** of those lacked proper AI access controls (IBM 2025). [5]
- Gartner predicts **40%** of enterprise apps will include task-specific AI agents by end of 2026, up from under 5% in 2025. [11]

## Why existing approaches fail

1. **Backups can be deleted by whoever holds admin rights**, whether a stolen credential or an over-privileged agent.
2. **Backups faithfully copy the damage.** Restoring "latest" restores the encrypted or corrupted state.
3. **Restore is all-or-nothing.** Rolling back a whole database to undo one agent's mistakes also throws away hours of legitimate human work.
4. **Nobody tests restores** until the worst day.

## Assumptions we have NOT proven yet

These are hypotheses. [03 · User research](03-user-research.md) exists to test them.

- H1: Finding the last clean recovery point after an incident takes **hours to days**. *(No statistic found.)*
- H2: Teams running AI agents against production data **have no per-agent undo** today.
- H3: Admins would accept a short delay on destructive agent actions in exchange for safety.

## How we will know we succeeded

Draft targets, to be finalised in the PRD:
- Time to identify a clean recovery point: **under 5 minutes**.
- Detection on simulated attacks: catch **at least 95%**, with **under 2%** false alarms on normal business changes.
- **100%** of restored files are cryptographically verified.
- Undo of one agent's changes leaves **0** human changes lost.
- Routine hourly backup uploads **under 5%** of the data size.

## Out of scope

- Preventing the initial break-in, and endpoint antivirus.
- Data theft / exfiltration. Backups cannot un-steal data. Sophos found 44% of attacks are now stopped before encryption, so theft-only extortion is a real and growing case we deliberately do not solve.
- Ransom negotiation.

## Sources and how much to trust them

Veeam, Sophos and Cyera sell security or backup products, so they have an incentive to make the problem look big. Verizon, IBM and Chainalysis are less directly invested. We cite several of each.

1. [Paubox – Change Healthcare costs](https://www.paubox.com/blog/change-healthcare-ransomware-attack-projected-to-cost-2.3-billion)
2. [ACS Information Age – AI agent deletes company database](https://ia.acs.org.au/article/2026/gone-in-9-seconds--ai-agent-deletes-company-database.html)
3. [Help Net Security – Verizon DBIR 2026](https://www.helpnetsecurity.com/2026/05/20/verizon-2026-dbir-findings/)
4. [The Record – Chainalysis 2025 ransomware payments](https://therecord.media/ransomware-payments-chainalysis-cybercrime)
5. [IBM Newsroom – Cost of a Data Breach 2025](https://newsroom.ibm.com/2025-07-30-ibm-report-13-of-organizations-reported-breaches-of-ai-models-or-applications,-97-of-which-reported-lacking-proper-ai-access-controls)
6. [Object First – Veeam Ransomware Trends 2025 summary](https://objectfirst.com/blog/summary-of-the-veeam-ransomware-trends-report-2025/)
7. [TechEconomy – Sophos State of Ransomware 2025](https://techeconomy.ng/10-key-findings-from-sophos-state-of-ransomware-report-2025/)
8. [Sophos – Impact of compromised backups](https://www.sophos.com/en-us/blog/the-impact-of-compromised-backups-on-ransomware-outcomes)
9. [Kinetic Consulting – April 2026 backup exploit campaign](https://www.kineticcg.com/blog/when-backup-becomes-the-target-what-the-april-2026-veeam-exploit-campaign-reveals-about-the-next-evolution-of-ransomware)
10. [Cyera Research – Agent-Inflicted Damage](https://www.cyera.com/research/agent-inflicted-damage-inside-the-real-world-failures-of-enterprise-ai-systems)
11. [Gartner – 40% of enterprise apps with AI agents by 2026](https://www.gartner.com/en/newsroom/press-releases/2025-08-26-gartner-predicts-40-percent-of-enterprise-apps-will-feature-task-specific-ai-agents-by-2026-up-from-less-than-5-percent-in-2025)
