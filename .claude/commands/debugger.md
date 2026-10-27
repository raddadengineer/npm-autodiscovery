---
description: Root-cause debugging, minimal reproduction scripts (repro.sh), and git bisect
---

Execute the Universal Root-Cause Debugger procedure:
1. Analyze and deobfuscate stack traces using `python3 scripts/bisect_debugger.py --analyze-trace <log>`.
2. Generate a minimal reproduction script using `python3 scripts/bisect_debugger.py --repro "<cmd>" --output repro.sh`.
3. If investigating a regression, run automated git bisect using `python3 scripts/bisect_debugger.py --bisect --test "<cmd>" --good <tag> --bad HEAD`.
4. Apply the surgical fix and verify Red-Green proof (exit 0).
