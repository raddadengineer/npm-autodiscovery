---
description: Run session closer handover, record in SESSION_LOG.md, and sync context files
---

Execute the Universal Session Closer procedure:
1. Analyze accomplishments via `git status --short` and `git diff --stat`.
2. Record a structured entry in `SESSION_LOG.md` documenting Goal, Changes, Validation, and Next Steps.
3. Update and sync context files (`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`).
4. Verify build and test suite under the Zero False Verification rule (must return exit code 0).
5. Deliver an executive handover summary.
