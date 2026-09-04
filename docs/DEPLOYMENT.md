# Production deployment

Native binaries are the supported Darkphish 0.2 release artifacts. Each release
contains SHA-256 checksums, an SPDX SBOM, and provenance. Historical Docker files
remain optional and receive no required CI or publishing guarantee.

## Required production keys

Supply independent 32-byte session keys and a versioned secret keyring:

```text
DARKPHISH_SESSION_AUTH_KEY=base64:<32-byte-value>
DARKPHISH_SESSION_ENCRYPTION_KEY=base64:<32-byte-value>
DARKPHISH_SECRET_ACTIVE_KEY=V1
DARKPHISH_SECRET_KEY_V1=base64:<32-byte-value>
```

Raw, `base64:`, and `hex:` encodings are accepted. Keep key IDs stable and use
only letters, digits, dots, underscores, or hyphens. Never commit key material.
Production startup fails when the active key is absent or invalid.
For 0.2 compatibility, the deprecated `DARKPHISH_SECRET_ENCRYPTION_KEY`
automatically becomes the active `legacy` key; migrate to the explicit keyring
variables before the compatibility fallback is removed.

To rotate without downtime, deploy the old and new keys together, select the new
ID with `DARKPHISH_SECRET_ACTIVE_KEY`, then run:

```sh
darkphish --config /etc/darkphish/config.json --rotate-secrets
```

Keep old keys available until the command succeeds on all shared databases and
backups using the old key have expired. The legacy
`DARKPHISH_SECRET_ENCRYPTION_KEY` may be supplied temporarily to read v1
envelopes that predate key IDs.

## Runtime and data

Place the administrative endpoint behind TLS, restrict it to administrators,
and isolate the simulation listener. Configure exact, scheme-qualified
`trusted_origins`; configure administrative `cors_allowed_origins` only where a
separate trusted client requires them. Use `/healthz` for process liveness and
`/readyz` for database readiness.

SQLite needs exclusive, filesystem-consistent backups of the configured DB file.
MySQL needs transactionally consistent backups and protected binary logs. Test
restoration before upgrades. Apply the configured audit retention and campaign
credential retention to replicas, snapshots, and off-site backups as well.

`audit.retention_days` defaults to 365. `personal_access_tokens.max_lifetime_days`
defaults to 90. Review these limits against organizational policy.
