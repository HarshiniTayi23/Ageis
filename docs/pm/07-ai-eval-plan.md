# 07 · AI evaluation plan

**Status:** v0.1. Part A has real numbers; part B is a plan · **Last updated:** 2026-11-12

*The single most important AI-PM skill: deciding how you'll know the AI is good enough before you ship it, and keeping it from quietly getting worse.*

## A. The ransomware / anomaly detector

**Dataset:** 440 labelled snapshots, built by `cmd/aegis-eval`:
- 22 scenarios × 20 seeds; 11 clean, 11 attack.
- Hard cases on both sides.
- A held-out set written before tuning.

Full method and results: [docs/eval](../eval/README.md).

**Which error is worse?**
- A **missed attack** means `--latest-clean` might restore an infected snapshot. In practice, `verify` and the next backup usually expose it.
- A **false alarm** costs an admin's attention. After a few of them, Ravi mutes alerts and recall becomes worthless, as the persona doc predicted.

So **precision is guarded with a hard limit, and recall is maximised within it.** That's why the PRD's counter-metric (false alarms ≤ 2%) beat a version of the detector with better raw recall.

| Metric | Ship bar | Current (v2) | Status |
|---|---|---|---|
| Recall, tuning set | ≥ 85% | 87% | ✅ |
| False-alarm rate, tuning set | ≤ 2% | 0% | ✅ |
| False-alarm rate, held-out set | ≤ 2% | 0% | ✅ |
| Recall, held-out set | ≥ 95% (PRD target) | 68% | ❌, the honest number |

**Reading the gap:** recall is 87% on scenarios I tuned for and 68% on ones I didn't. The 19-point drop is the expected cost of a hand-built detector. The PRD's 95% target is **not met**, and won't be until the detector learns from history (M3) and is measured on data its author didn't design.

**Guarding against regressions:** CI runs the evaluation with `-gate` on every push. The build fails if recall drops below 85% or any false-alarm rate rises above 2%. Held-out recall is deliberately not gated, so there's no temptation to tune against it.

## B. The LLM incident copilot (M3, not built yet)

**What it must never do**
- Claim a snapshot is clean when the detector flagged it.
- Invent file names, times or counts.
- Recommend restoring a snapshot newer than the incident.

**Design that makes this checkable:** the copilot gets structured facts (snapshot list, verdicts, reasons, drill results) and must cite a fact ID for every claim. A deterministic checker verifies each citation exists and says what the sentence claims. Uncited or mismatched sentences are dropped before display.

| Metric | Ship bar |
|---|---|
| Unverifiable claims shown to the user | 0 across the evaluation set |
| "What should I restore?" answered with the correct snapshot | 100% (it must match `--latest-clean`) |
| Usefulness, rated by a human on a 1–5 scale | ≥ 4 average |

**Evaluation set:** the 440 detector scenarios already produce realistic incident timelines. Pick 30 covering clean, attack and ambiguous cases, and write a reference summary for each by hand.

**Rollout:** show the copilot's summary *next to* the raw facts at first, labelled as a summary, never instead of them. Only consider making it primary after it clears the ship bar on the evaluation set *and* on a few real-looking incidents reviewed by someone else.
