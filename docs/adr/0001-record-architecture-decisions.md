# ADR-0001: Record architecture decisions

**Status:** Accepted · **Date:** 2026-11-04

## Context
Aegis is a learning project. In interviews the most common question isn't "what did you build" but "why did you build it that way, and what else did you consider?" Decisions made in chat or in someone's head are forgotten within weeks.

## Decision
Every significant technical decision gets a short Architecture Decision Record in `docs/adr/`, numbered in order, using `0000-template.md`. ADRs are never edited after acceptance. A changed decision gets a new ADR that supersedes the old one.

## Consequences
- A written trail of trade-offs to talk through in interviews.
- Small overhead per decision, kept low by the one-page template.
