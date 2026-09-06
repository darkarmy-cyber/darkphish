# Changelog

All notable changes to Darkphish are documented here. The project follows
Semantic Versioning while the public API and schema are still pre-1.0.

## 0.3.0 - 2026-09-05

### Added

- Added provider-neutral v3 envelope encryption with HashiCorp Vault Transit wrapping, local v1/v2 read compatibility, fail-closed provider handling, and resumable cross-provider re-encryption tooling.
- Added first-class PostgreSQL configuration, migrations, pooled runtime support, TLS verification policy, and hosted migration/security-model CI coverage alongside SQLite and MySQL.

### Changed

- Sensitive multi-write operations now use focused repositories, service boundaries, database transactions, and a durable audit outbox so partial credential, reviewer, token, user-role, or audit state is rolled back safely.
- Restore secret scanning with SHA-pinned Trivy Action v0.36.0, grant CodeQL the private-repository Actions read permission, and exercise bootstrap, nullable timestamps, PAT revocation, signed audit round trips, and injected transaction failures on all three databases.
- Make release preparation restartable with protected-main preflight, verification of generated branch history, safe fast-forward refresh, PR reuse, and explicit CI dispatch using GitHub's ephemeral token.
- Gate native publication on the protected release merge and all required checks; retain incomplete drafts for recovery, compare existing asset digests before reuse, and never overwrite release tags or binaries.
- Record standalone-repository ownership, required checks, single-maintainer protection, conservative dependency updates, and private-plan provenance limitations.

### Fixed

- Non-retainable repeat credential submissions now atomically purge any older ciphertext for the same result, preventing stale plaintext from remaining revealable.
- Runtime configuration now resolves each database prefix to its real `migrations` directory, so native binary startup and maintenance commands apply the packaged schema history.
- Compute audit hashes from the persisted database representation and sign checkpoint timestamps at portable microsecond precision so MySQL/PostgreSQL timestamp rounding cannot invalidate an untampered chain.
- Handle already-green release and maintenance pull requests with GitHub's protected merge-when-ready operation, pinning the exact head commit and retaining every check, review-conversation, and no-bypass requirement.
- Store absent user/IMAP last-login and campaign completion/send-by timestamps as SQL NULL across SQLite, strict MySQL, and PostgreSQL; show no last-login date for accounts that have never authenticated.
- Reconcile native publication on an independent schedule when GitHub suppresses downstream completion events from bot-dispatched checks, without relaxing protected-source, release-PR, or required-check gates.
- Default required modification timestamps in the persistence layer for groups, templates, pages, and mail settings, including callers outside HTTP handlers, without changing strict MySQL validation or existing real timestamps.
- Normalize legacy zero optional dates in every campaign-summary API while preserving real historical timestamps.
- Select release metadata from validated changelog targets, preserve the reserved manual patch-release path, and prevent preparation runs from replacing pending publication runs.

### Deprecated

- Legacy GOPHISH_* environment fallbacks and the single encryption-key path now emit bounded warnings; new migrate-check and secret-status/migrate commands identify and remediate compatibility hazards without exposing values.

### Security

- Credential reveal from a browser now requires a fresh, server-side, session-bound password reauthentication window in addition to account state, campaign access, RBAC, and credential permission checks.
- Added the narrow Security Reviewer role with expiring per-campaign assignments, centrally enforced access, audited assignment lifecycle, and no campaign ownership or administrative authority.
- Audit records now use deterministic SHA-256 hash chaining, rotating Ed25519-signed checkpoints, retention anchors, chain verification, and signed/verifiable JSON export manifests.
- Resolve initial-administrator password files independently of network database DSNs, require explicit production bootstrap configuration, create owner-only files without overwriting existing files, and redact all filesystem diagnostics.

### Migration

- Added reversible, data-preserving SQLite and MySQL 0.3 upgrade migrations plus a complete PostgreSQL fresh-install history, with up/down/up schema checks for all three backends.

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
