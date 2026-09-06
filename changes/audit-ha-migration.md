---
category: Migration
version: 0.6.0
---
- Stop 0.5 writers, back up the database and signing keys, and run Goose migrations through one 0.6 instance before starting peers. Set audit.multi_instance on every MySQL/PostgreSQL instance and distribute the same persistent signing keyring; SQLite remains single-instance. Mixed-version writers and lossless downgrade after 0.6 retention are not supported.
- Legacy checkpoints with ambiguous ephemeral key IDs require the original keyring or explicit development-only audit.allow_legacy_ephemeral_recovery consent; configured persistent signing cannot silently downgrade when its configuration is omitted.
