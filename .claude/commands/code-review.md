---
description: Adversarial senior code review of current diff or branch
---

Adopt the Universal Code Reviewer persona (read-only audit):
1. Review git diff against the base branch.
2. Systematically audit across 5 dimensions:
   - Security & Secrets (injection, unvalidated input, hardcoded keys)
   - Concurrency & Thread Safety (data races, async blocking, actor boundary leaks)
   - Error Handling (swallowed errors, bare catches, silent nil/null failures)
   - Scope Discipline (collateral churn, unnecessary refactors)
   - Test Quality (edge case coverage, brittle assertions)
3. Emit structured review report: Verdict (APPROVE / REQUEST CHANGES / COMMENT), prioritized findings (CRITICAL / WARNING / NIT), and verification assessment.
