---
description: Synthesize hardened GitHub Actions CI pipelines and audit workflow security
---

Execute the Universal CI/CD Workflow Engineer procedure:
1. Run `python3 scripts/synthesize_ci.py --audit --check` to scan `.github/workflows/` for unpinned actions, missing timeouts, and broad permissions.
2. Or scaffold a hardened pipeline via `python3 scripts/synthesize_ci.py --init --stack <python|node|go|rust>`.
