# Changelog

All notable changes to Darkphish are documented here. The project follows
Semantic Versioning while the public API and schema are still pre-1.0.

## 0.2.0 - 2026-09-04

### Added

- Campaign credential modes: disabled, irreversible policy-only evaluation, and explicitly authorized encrypted review.
- Versioned AES-256-GCM secret envelopes, active key IDs, online re-encryption, and automated credential retention cleanup.
- Scoped, expiring, revocable personal access tokens whose raw value is shown once and never stored.
- Persistent, indexed, filterable administrative audit records with audited CSV/JSON export.
- Changelog fragments and protected-branch release preparation for native binaries, checksums, SBOM, and provenance.

### Security

- Added policy-only and explicitly authorized encrypted credential-review modes, per-result reveal authorization, bounded retention, scoped expiring personal access tokens, Argon2id password hashing, versioned secret-key rotation, and persistent security audit records.
- Password hashes now use Argon2id; successful legacy bcrypt logins are upgraded in place.
- Credential reveal requires both `credentials:view` role permission and `credentials:view` PAT scope, uses a dedicated POST endpoint, defaults to masked state, and is excluded from normal APIs, webhooks, and exports.
- Legacy permanent API keys are disabled during migration.

### Changed

- Docker is no longer part of required CI or the release path; supported release artifacts are native binaries.
- The CLI reports the display version, authoritative SemVer, commit SHA, and build timestamp; local builds identify themselves as `0.2-dev`.
- The deprecated single `DARKPHISH_SECRET_ENCRYPTION_KEY` remains accepted for one release and is mapped to the `legacy` key ID; operators should migrate to the active-key/keyring variables before rotating.

### Breaking

- Existing permanent API keys stop authenticating. Create a scoped personal access token in Account Settings.

## 0.1.0 - 2026-09-04

### Added

- Darkphish module, binary, product identity, configuration defaults, and assets.
- Persistent production session keys and AES-256-GCM integration-secret storage.
- Request IDs, structured audit events, health/readiness endpoints, explicit CORS,
  administrative security headers, HTTP timeouts, and request-body limits.
- Resource limits for Office attachment templating.
- Go 1.27.1, deterministic pnpm frontend builds, hardened multi-stage container,
  PR CI, CodeQL, dependency updates, scanning, and release-candidate automation.

### Security

- Locked or password-reset-required accounts can no longer use API credentials.
- Query-string API authentication is rejected and API credentials are removed
  from browser-rendered state.
- Non-administrators cannot clear account lifecycle or role controls.
- SMTP, IMAP, and webhook secrets are redacted from API responses and encrypted
  on write when the required production key is configured.
- Credential form values are no longer retained; only submitted field names are
  recorded for awareness measurement.
- Internal-network and metadata destinations are denied by the restricted dialer
  while explicit administrator allowlists continue to work.

### Fixed

- Campaign result state advances only after its matching event is persisted.
- Campaign events use deterministic `time ASC, id ASC` ordering.

### Breaking

- External API callers must send `Authorization: Bearer`; `?api_key=` is rejected.
- Campaign completion now uses a cross-origin-protected `POST` instead of a state-changing `GET`.
- Administrative CORS is disabled unless exact origins are configured.
- Passwords must contain at least 12 characters.
- Production startup now fails without persistent session and integration-secret keys.
- Credential submission values are discarded even for legacy pages configured to
  capture passwords.
- Runtime names and defaults changed from Gophish to Darkphish. Transitional
  `GOPHISH_*` secret environment variables remain deprecated fallbacks for one release.
