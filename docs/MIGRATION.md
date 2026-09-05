# Migration to Darkphish 0.3

Back up the database, configuration, certificates, and runtime data before
starting. Darkphish applies database-specific Goose migrations in place for
SQLite and MySQL. PostgreSQL is supported for new 0.3 installations through a
consolidated upstream baseline followed by the Darkphish 0.1, 0.2, and 0.3
migrations. Cross-engine conversion is not automatic; use a validated ETL and
restore plan if changing database engines.

The 0.3 migrations add privileged sessions, campaign reviewer assignments, the
durable audit outbox, audit-chain fields, signed checkpoint records, and the
indexes needed by credential review and audit verification. Existing campaigns,
policy findings, encrypted credentials, PATs, and audit rows are preserved.
On first 0.3 startup, legacy 0.2 audit rows are deterministically linked in ID
order before new records are accepted.

## Required operator actions

1. Run `darkphish --config config.json migrate check`. Resolve reported database,
   certificate, environment-variable, API-key, and plaintext-secret hazards. The
   command is read-only and never prints environment values.
2. Configure persistent session keys, an envelope-key provider, and persistent
   audit-signing keys as described in `DEPLOYMENT.md`. Keep every old integration
   key required by live data or retained backups.
3. Start one upgraded instance and confirm migration success before upgrading
   the rest of a shared deployment.
4. Create scoped, expiring PATs for API clients. Migration disables values in
   the legacy `users.api_key` column; old permanent tokens stop authenticating.
5. Review every campaign. Existing and new campaigns default to credential mode
   `disabled`. Enable `policy_only` first unless retained plaintext review has a
   documented, authorized need.
6. Create dedicated Security Reviewer accounts and assign only the required
   campaigns with short expiries. Campaign ownership no longer implies credential
   reveal permission; browsers require a fresh five-minute reauthentication.
7. Run `darkphish secrets status`, then `darkphish secrets migrate`. Verify
   SMTP/IMAP/webhook connectivity and a synthetic credential before removing old
   keys. Migration is per-record, resumable, and idempotent.
8. Run `darkphish audit verify`, create a signed export, and verify it with
   `darkphish audit verify-export <export> <manifest>`.
9. Verify `/healthz`, `/readyz`, PAT scope enforcement, audit paging/export, and
   an authorized non-production campaign.

Historical credential values discarded by prior Darkphish versions remain lost.
The migration does not and cannot recover them. Historical integration secrets
remain readable only when their original key is retained; losing a key makes its
ciphertexts unrecoverable.

SQLite, MySQL, and PostgreSQL migrations represent the same Darkphish security
model with backend-appropriate auto-increment, boolean, timestamp, index, and
text/blob syntax. Each backend is exercised fresh-to-latest and latest-down/up;
MySQL and PostgreSQL additionally run the core security model in CI.

All `GOPHISH_*` fallback variables now emit one bounded warning per variable name
without its value. Compatibility is retained in 0.3 but scheduled for a future
removal. The single-key fallback remains readable only to support migration to a
versioned local keyring or external provider. Legacy permanent `users.api_key`
values remain disabled and cannot be restored as an authentication mechanism.

Before rollback, make a consistent database backup and retain all configured
envelope and audit keys. Rolling the 0.3 schema migration down removes only the
new 0.3 tables/columns/indexes, but a 0.2 binary cannot validate 0.3 chain data or
v3 Vault envelopes. Prefer restoring the tested pre-upgrade backup over running a
production rollback after new security data has been written.
