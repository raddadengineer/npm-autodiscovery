---
description: Calculate SemVer version bumps from Conventional Commits and generate CHANGELOG.md
---

Execute the Universal Release Manager procedure:
1. Run `python3 scripts/manage_release.py --bump` to calculate the next Semantic Version from git commit history.
2. Run `python3 scripts/manage_release.py --changelog` or `--notes` to update `CHANGELOG.md` and draft GitHub release notes.
