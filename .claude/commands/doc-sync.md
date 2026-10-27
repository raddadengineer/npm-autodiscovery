---
description: Documentation and schema drift sentinel to catch outdated docs and broken links
---

Execute the Universal Documentation Drift Sentinel procedure:
1. Run `python3 scripts/check_doc_drift.py --check`.
2. Verify all agents, skills, and tools on disk are accurately documented.
3. Fix any broken internal markdown references or missing descriptions.
