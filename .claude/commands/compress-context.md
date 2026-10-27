---
description: Profile repository token footprint and generate compressed AST context map
---

Execute the Universal Context Optimizer procedure:
1. Run `python3 .agents/skills/context-optimizer/scripts/compress_context.py --profile` to identify token consumption.
2. If preparing codebase context, generate an AST skeleton map (`--hybrid` or `--skeleton`) to minimize token usage by 85-95%.
3. Ingest only the compressed map to locate the precise target files before reading full implementations.
