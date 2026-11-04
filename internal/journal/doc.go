// Package journal is the second ingest path into the vault: an append-only
// record of every change an AI agent makes to company data, so that one
// agent's actions can be inspected and rolled back without touching human
// work.
//
// Status: planned for Milestone 2. See docs/pm/03-prd.md and
// docs/adr/0002-option-c-shared-recovery-core.md.
package journal
