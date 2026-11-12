# 02 · Personas

**Status:** Draft v0.2. Every line marked *(assumption)* must be confirmed or killed in [03 · User research](03-user-research.md). · **Owner:** Sri Harshini Tayi · **Last updated:** 2026-11-12

A persona is a specific person in a specific situation, not a demographic. The test of a good one: two engineers disagree about a feature, and they can settle it by asking "what would *this person* need at 3 a.m.?"

---

## P1 · Ravi, infrastructure lead at a 400-person logistics company · *primary persona for M1*

**Situation:** 03:10 on a Sunday. Monitoring alerts wake him; every file share shows a `HOW_TO_DECRYPT.txt`. He is one of three people in IT. *(assumption: small IT teams carry most of this pain. Sophos 2025 found 63% of victims cited a lack of staff or skills.)*

**Job to be done:** "When we've been hit, I want to know *which* backup is safe and get the business running again, so that I can tell my boss the restore will work and we don't need to pay."

**What he cares about, in order:**
1. **Certainty:** the restore point is genuinely clean.
2. **Speed:** trucks don't move while systems are down.
3. **Simplicity:** one command, at 3 a.m., while exhausted.

**What he fears:** restoring, then finding the restore is also encrypted, and losing another day. Being the person who says "the backups are gone."

**What he uses today, and why it fails him:** nightly backups to a NAS managed with the same admin account as everything else *(assumption)*. An attacker with that account can delete them. To find a clean point he opens backups one by one *(assumption H1: this takes hours to days)*.

**What would make him trust us:** a clear reason for every verdict ("80% of files jumped to random-looking content at 02:14"), and proof that restored files match the originals.

**What would make him turn us off:** false alarms. If Aegis cries "ransomware" every month-end close, he'll mute it.

**Quote:** _(add a real one from an interview)_

---

## P2 · Meera, CISO at the same company · *secondary for M1*

**Situation:** 06:00 the same morning. The CEO asks: "Do we pay?" The attackers want ₹8 crore and have given a 72-hour deadline.

**Job to be done:** "When we're under extortion, I want hard evidence of whether we can recover without paying, so that I can make the pay-or-restore call and defend it to the board."

**What she cares about, in order:**
1. **Evidence she can defend:** a recovery point with a timestamp and integrity proof.
2. **Time:** how many hours until the business is back.
3. **Assurance before an attack:** proof that restores work *now*, not just a promise.

**What she fears:** paying and not getting the data back. Veeam 2025 found 17% of payers never recovered. Or being attacked again: 69% of payers were. Regulators and customers asking why controls failed.

**What she uses today, and why it fails her:** reports from the backup tool that say "job succeeded", which prove a copy was made, not that it's clean or restorable *(assumption)*.

**What would make her trust us:** a regularly scheduled, verified test restore with a report she can show auditors.

**What would make her turn us off:** anything that needs Ravi to be a specialist to operate.

---

## P3 · Arjun, platform engineer at a 60-person SaaS startup · *primary persona for M2*

**Situation:** the team gave AI coding agents access to staging and, "temporarily", production. Last month an agent dropped a table it thought was a test fixture *(scenario based on the PocketOS incident, May 2026)*.

**Job to be done:** "When an agent does something wrong, I want to undo exactly what it did and nothing else, so that we keep the speed of agents without betting the company on them."

**What he cares about, in order:**
1. **Precise undo:** roll back one agent's changes without losing the humans' work from the same hour.
2. **Not slowing agents down:** guardrails must be nearly invisible on normal work.
3. **Visibility:** what did each agent touch, with which credentials?

**What he fears:** a nine-second deletion that takes three months of data with it. Being told to ban agents entirely.

**What he uses today, and why it fails him:** database snapshots every few hours *(assumption H2)*. Restoring one rolls back *everyone's* changes, not just the agent's. Backups often sit on the same volume the agent can delete.

**What would make him trust us:** a dry run: "here's exactly what undoing agent X would change" before it changes anything.

**What would make him turn us off:** approval prompts on every action. *(assumption H3: a short delay on destructive actions only is acceptable.)*

---

## Where these personas conflict

| Tension | Ravi / Arjun want | Meera wants | Our default |
|---|---|---|---|
| Alert sensitivity | Few alerts, only real ones | Catch everything | Optimise for precision; Meera gets a daily digest of near-misses |
| Agent guardrails | Agents never blocked | Every destructive action reviewed | Delay only destructive actions; everything else is just recorded |
| Detail | One command, one answer | Full evidence trail | Simple output by default, evidence available on demand |

## Provisional decisions (v0.2, 2026-11-12)

These were open questions. They're answered provisionally so building can continue. Each can be overturned. See `docs/BUILD_LOG.md`.

1. **Is Ravi primary for M1?** Yes. He's the one who runs the restore, and every M1 capability is a restore capability. If Meera were primary, the first feature would be *proof*, not *recovery*: scheduled test restores with an audit report. That idea scored well enough in the roadmap's RICE table that it's now in M1 as `aegis drill` (requirement R7 in the PRD).
2. **Do Rubrik's customers look like Ravi's company?** Mostly bigger: thousands of employees, many sites, separate backup and security teams. Three implications, all already on the roadmap:
   - More data, which justifies the distributed catalog (M3).
   - More than one admin, so no single person should be able to shorten retention. That's handled by COMPLIANCE-mode Object Lock in M1.
   - Separate teams, so the alert must explain itself to someone who didn't configure it.
3. **Which assumption would hurt most if wrong?** For M1 it's **H1**, that finding a clean recovery point takes hours to days. If teams already find it in minutes, `--latest-clean` is a nice-to-have, and the pitch would shift to immutability and proof. For M2 it's **H3**, that a short delay on destructive agent actions is acceptable. Both are the first questions in the interview script.
