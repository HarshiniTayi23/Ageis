#!/usr/bin/env bash
# Turns docs/pm/05-roadmap.md into GitHub milestones, labels and issues.
# Run once after creating the repo:  gh auth login && ./scripts/bootstrap-github.sh
# Safe to re-run: existing milestones and labels are skipped.
set -euo pipefail
REPO="${REPO:-HarshiniTayi23/Ageis}"

milestone() { gh api "repos/$REPO/milestones" -f title="$1" -f description="$2" >/dev/null 2>&1 || true; }
label()     { gh label create "$1" --repo "$REPO" --color "$2" --description "$3" 2>/dev/null || true; }
issue()     { gh issue create --repo "$REPO" --milestone "$1" --label "$2" --title "$3" --body "$4" >/dev/null && echo "  + $3"; }

echo "labels"
label story    1d76db "User-facing capability"
label pm       8250df "Product management document"
label infra    0e8a16 "Build, deploy, CI"
label security b60205 "Security property"
label ai       fbca04 "AI / ML feature"

echo "milestones"
milestone "M1 · Ransomware vault"     "Recover the last clean copy after ransomware, from storage the attacker cannot delete."
milestone "M2 · Agent undo"           "See what one AI agent changed and roll back only that."
milestone "M3 · Distributed + AI"     "Survive a node failure; grounded LLM incident summaries."

echo "issues"
M1="M1 · Ransomware vault"; M2="M2 · Agent undo"; M3="M3 · Distributed + AI"
issue "$M1" pm       "Write personas (docs/pm/02)"                 "Fill docs/pm/02-personas.md for the three starting personas. Decide the primary persona for M1."
issue "$M1" pm       "Run 2-5 user interviews (docs/pm/03)"         "Test hypotheses H1-H3. Record quotes and what changed."
issue "$M1" pm       "Write the PRD (docs/pm/04)"                   "Goals, non-goals, MoSCoW requirements, metrics with baselines and a counter-metric."
issue "$M1" pm       "Score the roadmap with RICE (docs/pm/05)"     "Fill the RICE table and adjust the milestone order if the scores disagree."
issue "$M1" security "S3 backend with Object Lock (COMPLIANCE)"     "Implement store.ObjectStore against S3 (MinIO locally) with per-object retention. Write ADR-0004 first."
issue "$M1" infra    "Run the k8s manifests on kind"                "Build the image, load into kind, apply deploy/k8s, confirm probes and CronJob work. Fix what breaks."
issue "$M1" pm       "Record demo video #1"                         "Two minutes: the problem, the attack, the detection, the clean restore."
issue "$M2" pm       "Competitive analysis (docs/pm/06)"            "Rubrik, Veeam, Cohesity, AI-agent guardrail tools."
issue "$M2" story    "Agent action journal"                         "Capture every write an AI agent makes, tagged with agent identity."
issue "$M2" security "Guardrails for destructive agent actions"     "Delay or require approval for DROP, unscoped DELETE, bucket/volume deletion."
issue "$M2" story    "Selective undo by agent"                      "Roll back one agent's changes without losing concurrent human changes."
issue "$M2" story    "Replay the PocketOS incident as a demo"        "Agent deletes DB + co-located backups; Aegis vault survives; undo restores."
issue "$M3" pm       "AI evaluation plan (docs/pm/07)"              "Datasets, metrics, ship bars for the detector and the copilot."
issue "$M3" infra    "3-node Raft catalog"                          "Replicate the snapshot catalog with hashicorp/raft; survive one node loss."
issue "$M3" ai       "ML anomaly detector with labelled eval set"   "Beat the rule-based detector on the held-out set without raising false positives."
issue "$M3" ai       "Grounded LLM incident copilot"                "Every claim checked against catalog data before it is shown."
issue "$M3" infra    "Prometheus metrics + dashboard"               "Backup duration, dedup ratio, anomaly score, alerts on suspicious snapshots."
echo "done: https://github.com/$REPO/issues"
