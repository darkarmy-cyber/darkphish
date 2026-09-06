---
category: Changed
version: 0.5.0
---
- Move all application persistence to GORM v2 and the maintained pgx-backed PostgreSQL driver; remove legacy GORM and lib/pq without changing the Goose-managed schema.
- Preserve explicit zero-value writes, nullable timestamp hooks, missing-record errors, ownership predicates and manually maintained associations; isolate reused query statements and propagate aggregate errors.
