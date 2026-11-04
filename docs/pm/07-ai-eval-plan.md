# 07 · AI evaluation plan

**Status:** Template · Due before Milestone 3 · *The single most important AI-PM skill: deciding how you'll know the AI is good enough before you ship it.*

## Two AI features to evaluate

### A. The ransomware / anomaly detector
- **Labelled dataset:** _how many clean scenarios, how many attack scenarios, which attack styles (rename storm, in-place encryption, slow encryption over days, wipers)?_
- **Metrics:** precision, recall, false-positive rate on normal business changes.
- **Which error is worse?** A missed attack (restore picks an infected point) vs a false alarm (admin learns to ignore alerts). _Decide and justify; it sets the threshold._
- **Ship bar:** _e.g. recall ≥ 95% and false-positive rate ≤ 2% on the held-out set._

### B. The LLM incident copilot
- **What it must never do:** claim a snapshot is clean when the detector flagged it; invent file names, times or counts.
- **Grounding check:** every factual claim in the summary must map to a catalog record. Automated check: _how?_
- **Eval set:** _N incident scenarios with a reference summary written by hand._
- **Metrics:** factual accuracy (claims verified), hallucination rate, usefulness (rated by a human: did it tell me what to restore?).
- **Ship bar:** _e.g. 0 unverified claims across the eval set._

## Rollout plan
_Would you show the copilot's output directly, or first as a "suggestion" next to the raw data? Why?_
