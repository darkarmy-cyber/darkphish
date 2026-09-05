# Production deployment

Native binaries are the supported Darkphish 0.3 release artifacts. Each release
contains SHA-256 checksums and an SPDX SBOM; attestations are included when the
private-repository plan supports them. Historical Docker files
remain optional and receive no required CI or publishing guarantee.

## Required production keys

Initial administrator setup requires `DARKPHISH_INITIAL_ADMIN_PASSWORD`,
`DARKPHISH_INITIAL_ADMIN_PASSWORD_FILE`, or `bootstrap_directory` in production.
The password value takes precedence. The password-file variable remains a
**generated output filename**, not an input secret-file reader. The configured
directory is relative to the config file; create and secure it before startup.
Files use Unix mode 0600 or an owner-only Windows DACL, refuse existing files and
symlinks, never appear in logs, and are removed after the forced password change.

Development may derive the directory from a genuine SQLite filename. Network
DSNs and SQLite URI forms instead use `darkphish_initial_admin_password` in the
working directory unless a destination is configured. SQLite `:memory:` creates
no file without an explicit destination. No directory is created from a DSN;
filesystem errors omit paths and underlying error text to prevent secret leaks.

Supply independent 32-byte session keys, a credential/integration key provider,
and a purpose-specific Ed25519 audit-signing seed:

```text
DARKPHISH_SESSION_AUTH_KEY=base64:<32-byte-value>
DARKPHISH_SESSION_ENCRYPTION_KEY=base64:<32-byte-value>
DARKPHISH_SECRET_ACTIVE_KEY=V1
DARKPHISH_SECRET_KEY_V1=base64:<32-byte-value>
DARKPHISH_AUDIT_ACTIVE_SIGNING_KEY=A1
DARKPHISH_AUDIT_SIGNING_KEY_A1=base64:<32-byte-Ed25519-seed>
```

Raw, `base64:`, and `hex:` encodings are accepted. Keep key IDs stable and use
only letters, digits, dots, underscores, or hyphens. Never commit key material.
Production startup fails when the active key is absent or invalid.
For compatibility, the deprecated `DARKPHISH_SECRET_ENCRYPTION_KEY`
automatically becomes the active `legacy` key; migrate to the explicit keyring
variables before the compatibility fallback is removed.

To rotate without downtime, deploy the old and new keys together, select the new
ID with `DARKPHISH_SECRET_ACTIVE_KEY`, then run:

```sh
darkphish --config /etc/darkphish/config.json secrets status
darkphish --config /etc/darkphish/config.json secrets migrate
```

Keep old keys available until the command succeeds on all shared databases and
backups using the old key have expired. The legacy
`DARKPHISH_SECRET_ENCRYPTION_KEY` may be supplied temporarily to read v1
envelopes that predate key IDs.

Audit signing keys are independent from session, database, and envelope keys.
During rotation configure old and new signing seeds together and change the active
ID; old public verification material must remain available for existing
checkpoints and manifests. Run `darkphish audit verify` after rotation.

## Vault Transit

Set `secrets.provider` to `vault` and configure `secrets.vault.address`, `mount`,
`key_name`, optional `namespace`/`ca_cert`, and a narrowly scoped token through
`DARKPHISH_VAULT_TOKEN` or `token_file`. Production requires HTTPS. Darkphish
generates a fresh local data key for each value and asks Transit to encrypt only
that key; Vault never receives the credential plaintext. Do not configure an
implicit fallback. During a local-to-Vault migration keep the local read keys and
Vault available, run `secrets migrate`, then confirm `secrets status` reports no
remaining local records before removing old keys.

## PostgreSQL

Set `db_name` to `postgres`, leave `db_path` empty, and use the structured block:

```json
"postgresql": {
  "host": "postgres.example.net",
  "port": 5432,
  "database": "darkphish",
  "username": "darkphish",
  "password_file": "/run/secrets/darkphish-postgres-password",
  "sslmode": "verify-full",
  "connect_timeout_seconds": 10
}
```

`DARKPHISH_POSTGRES_PASSWORD` overrides the password field/file. Production
rejects a raw `db_path` PostgreSQL DSN and TLS modes weaker than `verify-full`;
set `db_sslca_path` when the server certificate uses a private CA. Pool controls
are `db_max_open_conns`, `db_max_idle_conns`, and
`db_connection_max_lifetime_minutes`. SQLite remains single-connection by
default; server databases default to ten connections.

## Runtime and data

Place the administrative endpoint behind TLS, restrict it to administrators,
and isolate the simulation listener. Configure exact, scheme-qualified
`trusted_origins`; configure administrative `cors_allowed_origins` only where a
separate trusted client requires them. Use `/healthz` for process liveness and
`/readyz` for database readiness.

SQLite needs exclusive, filesystem-consistent backups of the configured DB file.
MySQL and PostgreSQL need transactionally consistent backups and protected
replication/WAL data. Test
restoration before upgrades. Apply the configured audit retention and campaign
credential retention to replicas, snapshots, and off-site backups as well.

A database backup containing encrypted values cannot be restored usefully without
the local envelope keys or access to the same external-provider key references.
Back up configuration and key material through the organization's secret-management
system, never inside a database dump. Preserve audit signing/verification keys
separately. A restore drill must start Darkphish, run `secrets status`, decrypt a
non-production integration secret, run `audit verify`, and verify the latest signed
export before the restored instance is trusted.

`audit.retention_days` defaults to 365. `personal_access_tokens.max_lifetime_days`
defaults to 90. Review these limits against organizational policy.
`privileged_access.window_minutes` defaults to five and is constrained to 1–15.
`audit.checkpoint_interval` defaults to 1,000 records.
