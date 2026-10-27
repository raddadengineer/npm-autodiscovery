---
description: SAST security scan, secret leak prevention, and dependency vulnerability audit
---

Execute the Universal Security Auditor procedure:
1. Run `python3 scripts/audit_security.py --check` across the repository.
2. Audit for OWASP Top 10 injection vectors (SQL, command execution, eval).
3. Verify zero hardcoded secrets or exposed private keys.
4. Report prioritized findings (CRITICAL / HIGH / MEDIUM).
