---
description: Database migration safety, zero-downtime checks, and rollback reversibility audit
---

Execute the Universal Database Migration Safety procedure:
1. Run `python3 scripts/verify_migrations.py --check` across all migration directories.
2. Verify all migrations have valid downgrade/rollback procedures.
3. Audit for table-locking anti-patterns (ADD COLUMN NOT NULL without default, non-concurrent indexes).
4. Verify query plans using EXPLAIN ANALYZE for hot paths.
