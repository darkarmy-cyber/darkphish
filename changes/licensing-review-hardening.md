---
category: Security
version: 0.11.0
---
- Harden the WordPress licensing service: invalidate sibling recovery links atomically, remove incomplete newly created signing files, reject signing keys beneath either public root, strip shortcode verification fragments before DOM readiness, require signing/storage readiness before public registration, and use an additive indexed canonical-email lookup that preserves legacy duplicate records.
