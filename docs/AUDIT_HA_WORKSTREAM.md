# Darkphish 0.6 audit consistency

Baseline: `e38c64c9a90c3f6b96063fb569249e31be8ae611` (`v0.5.0`).

This engineering workstream replaces process-local audit coordination with a
transactionally locked chain head. All chain mutations and consistent reads use
the same lock. Durable delivery receipts preserve OutboxID idempotency across
retention. SQLite remains a single-instance deployment option.

Release gates: separate-process MySQL/PostgreSQL contention tests; immutable
v0.5 compatibility fixtures; all protected checks and CodeQL; normal and security
review; protected engineering and generated release PRs; native release assets,
checksums, SPDX SBOM and released-binary multi-instance smoke.

Evidence is recorded on engineering PR #17 and the generated release PR. The
VERSION file advances to 0.6.0 through release preparation; the target tag is
v0.6.0. Publication requires the native archive smoke gate to pass.
