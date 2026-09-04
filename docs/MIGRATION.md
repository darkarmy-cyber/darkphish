# Migration to Darkphish 0.2

Back up the database, configuration, certificates, and runtime data before
starting. Darkphish applies Goose migrations in place for both SQLite and MySQL.
The 0.2 migration adds campaign policy controls, separate derived policy results,
encrypted review records, personal access tokens, and indexed audit events.

## Required operator actions

1. Configure persistent session keys and a versioned secret keyring as described
   in `DEPLOYMENT.md`. Keep the old integration-secret key during rotation.
2. Start one upgraded instance and confirm migration success before upgrading
   the rest of a shared deployment.
3. Create scoped, expiring PATs for API clients. Migration disables values in
   the legacy `users.api_key` column; old permanent tokens stop authenticating.
4. Review every campaign. Existing and new campaigns default to credential mode
   `disabled`. Enable `policy_only` first unless retained plaintext review has a
   documented, authorized need.
5. If encrypted review is approved, grant `credentials:view` only to the small
   review group, set short retention, and align database backups and replicas.
6. Run `--rotate-secrets`, verify SMTP/IMAP/webhook connectivity, then remove old
   keys only after backup retention permits.
7. Verify `/healthz`, `/readyz`, PAT scope enforcement, audit paging/export, and
   an authorized non-production campaign.

Historical credential values discarded by prior Darkphish versions remain lost.
The migration does not and cannot recover them. Historical integration secrets
remain readable only when their original key is retained; losing a key makes its
ciphertexts unrecoverable.

SQLite and MySQL migrations preserve campaign and policy findings when expired
credential ciphertext is cleared. No PostgreSQL or GORM major-version migration
is included in 0.2.
