# Darkphish 0.6 audit consistency

Baseline: `e38c64c9a90c3f6b96063fb569249e31be8ae611` (`v0.5.0`).

Implement database-backed audit coordination, durable OutboxID idempotency,
consistent checkpoints, verification, export and retention. Preserve historical
hashes and formats. Require separate-process MySQL/PostgreSQL evidence and
v0.5 migration compatibility before protected release.

Status: implementation in progress; no HA or release acceptance is claimed.
