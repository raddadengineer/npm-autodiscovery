---
description: Audit multi-ecosystem dependency manifests for wildcards, missing lockfiles, and deprecations
---

Execute the Universal Dependency Auditor procedure:
1. Run `python3 scripts/audit_deps.py --check` across the repository.
2. Remediate any wildcard version specifiers (`*`, `latest`), deprecated packages, or missing lockfiles.
