# Migration from the upstream baseline

Darkphish 0.1.0 starts from commit
`95618469799295e2c0fec980805a2dfbb818816b`. Existing SQLite and MySQL data is
preserved; startup applies normal Goose migrations and does not recreate the
database.

## Operator checklist

1. Back up the database, configuration, certificates, and runtime data.
2. Rename runtime paths and service references to `darkphish`. The default
   SQLite filename is now `darkphish.db`; explicitly retain an old path when
   migrating an existing database.
3. Replace new configuration keys from the supplied `config.json`. Production
   deployments must enable `production_mode` and supply persistent session and
   integration-secret keys.
4. Update external API clients to `Authorization: Bearer <token>`. URL query
   authentication now returns 401. Send `POST`, not `GET`, to the campaign
   completion endpoint.
5. Configure exact administrative CORS origins only if required. CSRF
   `trusted_origins` must also be exact and scheme-qualified (for example,
   `https://admin.example.com`); production rejects plaintext origins.
6. Verify SMTP, IMAP, and webhook settings. Existing plaintext values can be
   read for compatibility and are encrypted on their next write when the
   encryption key is configured.
7. Ensure users know the 12-character minimum password policy.
8. Validate custom landing pages: submitted field names are recorded, but values
   and passwords are always discarded regardless of legacy capture flags.
9. Start the service, verify `/healthz` and `/readyz`, then test a non-production
   authorized campaign before normal use.

## Compatibility identifiers

For one transitional release, the legacy `GOPHISH_SESSION_AUTH_KEY`,
`GOPHISH_SESSION_ENCRYPTION_KEY`, `GOPHISH_SECRET_ENCRYPTION_KEY`,
`GOPHISH_INITIAL_ADMIN_PASSWORD`,
`GOPHISH_INITIAL_ADMIN_API_TOKEN`, and initial-password-file variables are
accepted as deprecated fallbacks. `DARKPHISH_*` always takes precedence.

## Database migrations

The 0.1.0 data migration only updates built-in permission descriptions from the
upstream product name to Darkphish. It changes no tables or user data. Secret
encryption is deliberately lazy-on-write to preserve existing installations.

No PostgreSQL support or GORM major-version migration is included. Those require
a dedicated cross-backend migration and CRUD/campaign compatibility suite.
