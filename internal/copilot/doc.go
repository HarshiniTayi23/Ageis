// Package copilot will turn detector output and catalog history into a
// plain-language incident summary ("what happened, which recovery point is
// clean, what to restore") using an LLM, where every claim in the summary
// is checked against real catalog data before it is shown.
//
// Status: planned for Milestone 3. Evaluation plan: docs/pm/05-ai-eval-plan.md.
package copilot
