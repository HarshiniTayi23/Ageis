# 03 · User research

**Status:** Template · **Goal:** test hypotheses H1–H3 from the [problem statement](01-problem-statement.md) with 3–5 real people before we lock the PRD.

## Who to talk to
People who run infrastructure: sysadmins, DevOps/SRE engineers, IT support at a college or company, anyone who has had to restore a backup. Friends, seniors, LinkedIn, college alumni. Two conversations are far better than zero.

## Rules for the interview
- Ask about **past behaviour**, not opinions about the future. "Tell me about the last time you had to restore something" beats "Would you use a tool that…?"
- Don't pitch Aegis until the end, if at all.
- Write down exact quotes.

## Script (20 minutes)
1. What do you look after, and what backup tooling do you use?
2. Tell me about the last time you had to restore data. What happened, step by step?
3. How did you decide *which* backup to restore from? How long did that take? *(tests H1)*
4. Have you ever tested a restore when nothing was broken? Why or why not?
5. Do any AI tools or agents have write access to your systems? What can they change? *(tests H2)*
6. If one of them did something wrong, how would you undo just that? *(tests H2)*
7. Would you accept a short delay or approval step before an agent deletes something? Where's the line? *(tests H3)*
8. What would make you not trust a tool like this?

## Notes

| # | Who (role, company size) | Date | Key quotes | H1 | H2 | H3 |
|---|---|---|---|---|---|---|
| 1 | | | | ✅/❌/? | | |

## What we learned and what we changed
_After the interviews: which assumptions held, which broke, and what changed in the PRD as a result._
