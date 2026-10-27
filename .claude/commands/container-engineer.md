---
description: Scaffold hardened multi-stage Dockerfiles and audit container security & layer caching
---

Execute the Universal Container Engineer procedure:
1. Run `python3 scripts/harden_docker.py --audit --check` to inspect existing Dockerfiles.
2. Or scaffold a hardened, non-root, multi-stage Dockerfile via `python3 scripts/harden_docker.py --init --stack <python|node|go|rust>`.
3. Verify zero `:latest` base image tags, no baked secrets, and deterministic dependency layer caching.
