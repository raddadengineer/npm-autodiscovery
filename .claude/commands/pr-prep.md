---
description: Pre-flight audit for pull requests and generate PR description
---

Execute the Universal PR Preparation procedure:
1. Audit git diff for scope creep, leftover debug statements (`console.log`, `print()`, `debugger`), and potential credential leaks.
2. Verify all test suites and production builds pass cleanly.
3. Check that `SESSION_LOG.md` reflects current branch changes.
4. Generate a clean PR description with Summary, Key Changes, Test Verification receipts, and Checklist.
