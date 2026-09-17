# Changelog

## 0.13.0 - 2026-09-17

### Fixed

- Preserve waiting protected release and merge operations in a shared bounded queue, with strict workflow-policy validation and unchanged publication safeguards.
- Retire automatic historical release repairs in favor of explicitly confirmed main-only maintenance; exclude retained drafts from public metadata reconciliation without deleting or publishing them.
- Keep authenticated, non-required GitBook previews informational without bypassing required checks, code/security reviews, or protected exact-head merges.
- Reserve distinct loopback ports for native multi-process audit tests and preserve bounded, redacted startup diagnostics; retain concurrent MySQL/PostgreSQL writers and fail on any child startup failure.
- Use the existing DarkPhish fish-and-hook artwork for sign-in, password reset, navigation and favicons, with responsive authentication layouts and accessible input labels.
- Point User Guide and API Documentation to https://docs.darkphish.sk/.


## 0.12.0 - 2026-09-17

### Fixed

- Preserve the trusted release PR creation date in both validation and actual changelog generation, preventing date-only head changes across UTC midnight without relaxing review or publication checks.

## 0.11.0 - 2026-09-17

### Added

- Add the Community licensing protocol, signed lease verification, activation persistence, entitlement checks, and administrative license API foundation.
- Add an installable WordPress Community licensing service with verified registration, Ed25519 leases, transactional activation binding, administrative lifecycle controls, and integration tests.
- Allow an HTTPS administrator to initialize a private signing file outside the web root without SSH, with permission/CSRF checks and exclusive creation that never replaces an existing key.
- Support static HTML registration on an explicitly trusted HTTPS origin, with scoped credential-free CORS, hostname-bound Turnstile validation, public configuration and real HTTP/browser-flow tests.
- Add the Darkphish Licenses dashboard below Zeus, edition-filtered management, editable plan defaults, manual Professional/Enterprise issuance, signed edition claims and additive upgrade tests.

### Security

- Enforce Community licensing at server startup, serialize entitlement checks across database writers, reject caller-supplied creation IDs, and recheck license validity before queued campaign delivery.
- Reject licensing redirects and incomplete activation responses, preserve existing recipient identities in degraded mode, and add administrator activation controls with periodic signed-lease refresh.
- Distribute the maintainer-provided fsociety.sk public verification key and API configuration with native archives, the source installer and Docker, retaining license state on persistent storage.
- Accept authenticated Professional and Enterprise edition claims with the same installation, expiry and signed entitlement enforcement; reject unknown editions.
- Harden the WordPress licensing service: invalidate sibling recovery links atomically, remove incomplete newly created signing files, reject signing keys beneath either public root, strip shortcode verification fragments before DOM readiness, require signing/storage readiness before public registration, and use an additive indexed canonical-email lookup that preserves legacy duplicate records.
- Create Windows licensing state with a protected owner-only DACL before writing credentials, including every atomic replacement, so shared-directory permissions cannot expose refresh tokens. Unix file-mode protections remain unchanged.

### Migration

- Add the license coordination table without rewriting existing customer data; configure the approved licensing API, public verification keyring and persistent license state path before activating Community installations.

## 0.10.0 - 2026-09-15

### Added

- Add a hardened `./install.sh` for fresh systemd-based Linux deployments that verifies the host and pinned Go toolchain, builds only the exact tracked source snapshot, creates a dedicated service account, generates production security material, installs the native runtime, validates application readiness, and rolls back installer-created artifacts on failure.

### Fixed

- Harden the native Linux installer against inherited command-bearing environments, require systemd 245+ with a reachable manager, reject unusable `noexec` build locations before persistent mutation, and require stable readiness from both the administrative and simulation listeners before reporting success.
- Report merged releases without a tag or staging draft as blocked recovery, identify missing maintainer attestations directly, and require verified current-version publication before preparing any subsequent version.

### Security

- Require the existing exact-head maintainer release attestation before a generated release can merge, and revalidate it immediately before the protected merge. Missing, stale, forged, revoked or unresolved approval now stops the merge instead of stranding publication afterward.

## 0.9.0 - 2026-09-15

### Fixed

- Discover unpublished release drafts through authenticated, paginated release listings and immutable IDs; reject ambiguous or changed metadata and generate canonical notes before staging without replacing existing artifacts.

## 0.8.0 - 2026-09-15

### Added

- Centralize administrative navigation in Settings with permission-gated tabs and stable release notifications.
- Add reauthenticated native Linux/SQLite updates with release evidence verification, offline backups, supervised restart and application/database rollback. Other platforms and database engines report why one-click apply is unavailable.
- Require independently verified Sigstore workflow attestations before executing update payloads, using an administrator-installed GitHub CLI and embedded trust roots without GitHub credentials.

### Changed

- Configure GitBook Site Git Sync for the maintained documentation, contributor-agreement, and changelog-fragment directories.
- Updated github.com/mattn/go-sqlite3 from 1.14.32 to 1.14.52.
- Updated github.com/pressly/goose/v3 from 3.27.3 to 3.28.0 together with its synchronized migration-engine dependencies.
- Updated github.com/gorilla/context from 1.1.1 to 1.1.2.
- Updated golang.org/x/crypto to 0.57.0 together with its synchronized x/sys, x/sync, and x/text dependency set.

### Fixed

- Verify the single audited pre-attestation v0.7.0 release merge using its immutable PR/head/base/merge identity and both authentic exact-head code and security reviews completed before merge, instead of inferring provenance from merge-script substrings or accepting missing reviews. Keep modern maintainer attestation enforcement unchanged.
- Repair a trusted GitHub rollback draft that was automatically detached from its canonical release tag by metadata-only editing, while preserving the immutable source, draft state, title, canonical notes and exact release asset identities.
- Recognize the exact historical v0.7.0 release footer generated by its immutable source publisher during metadata reconciliation, while keeping the compatibility path bound to that source SHA and rejecting altered or additional release text.
- Trigger release-metadata reconciliation from either successful main CI or CodeQL completion so the later trust workflow can safely proceed once every required check for the exact protected-main SHA is green.
- Recognize CRLF-encoded trusted release descriptions as the same immutable source-derived metadata before canonicalizing them to the standard release-note format, without accepting content changes.
- Treat a withdrawn draft whose GitHub-generated URL is already `untagged-*` as detached during trusted tag repair even if the release-list `tag_name` field is stale, while retaining fail-closed checks for real tag conflicts.
- Serialize release reconciliation with publication workflows and enforce a trusted normalization hold so a verified recovery publication cannot race historical release metadata repair.
- Canonicalize every trusted GitHub release description to the v0.5.0 Changed, Fixed, Security, Migration structure while preserving release state, immutable tags, sources, and binary artifacts.
- Verify receipt-backed staged release artifacts from exact-green protected-main recovery runs and immutable source-bound attestations before reconciling release metadata.
- Restore the verified v0.7.1 release through a narrowly scoped, reviewed recovery path, preserving immutable tags, original attested artifacts and all publication checks.
- Validate GitHub-generated publication timestamp transitions while preserving original signed release artifacts and strict post-publication checks.

### Security

- Verify signed audit export readers one event at a time, require complete input and strict chain/checkpoint checks, and reject sequence overflow while preserving existing export formats and byte-array callers.
- Recognize the review connector's exact optional Resolved display suffix while preserving historical finding identities, fresh code/security reviews, provenance checks and independent resolved-thread verification before merging.

## 0.7.1 - 2026-09-13

### Fixed

- Report exact rule and source-location identities for trusted generated-release CodeQL results while preserving fail-closed behavior, so blocked releases can be diagnosed without suppressing findings.
- Preserve an existing protected IMAP password when administrators update other reporting settings without entering a replacement password.
- Recover generated release checks that GitHub marks `action_required` before any job executes, while preserving fail-closed behavior for real CI or CodeQL failures and bounding each exact-head recovery dispatch to a single attempt.
- Reproduce historical generated-release changelog dates from the existing GitHub release PR creation timestamp so safe release recovery remains deterministic across calendar days without trusting branch-controlled date content.
- Fetch release tags before native patch-release validation so the publication workflow recognizes the already-published current version instead of failing on a shallow checkout.
- Keep development-version regression tests valid when a generated patch release advances `VERSION` within the same minor line.

### Security

- Require authenticated, completed code and explicit security reviews of the current head and resolved review threads before automated engineering or generated-release merges; fail closed on stale, edited, ambiguous or unavailable evidence.
- Replace queued merge permissions with a single protected squash merge matching the complete head SHA, preserve label-removal pauses during release recovery, and recheck pre-merge review evidence before native publication.
- Reconcile trusted generated-release full-head CodeQL results only against the unchanged protected-main baseline for generated-only release PRs, preserving local SARIF evidence while continuing to fail closed on unresolved alerts, unexpected files, or target/base changes.
- Allow generated release publication to use an exact-head, exact-base maintainer approval from the repository owner instead of Codex review evidence, while preserving generated-file scope, protected-main checks, CodeQL baseline verification and fail-closed review semantics.

## 0.7.0 - 2026-09-07

### Changed

- Read audit event and checkpoint history in bounded keyset pages during startup, chain verification, retention, and signed export construction, preserving transactional chain coordination and existing hashes, signatures, and export bytes.
- Exercise page-boundary corruption, rollback, typed retry, legacy receipt backfill, and multi-page MySQL/PostgreSQL multi-process history without weakening audit verification. Signed export payloads remain buffered by the existing byte-array interface; full streaming export and shorter writer-lock duration remain future work.
- Extend native archive and downloaded-release audit smoke to 270 denied loopback requests, proving page traversal, exact durable coverage, restart and signed export on packaged binaries.

## 0.6.0 - 2026-09-06

### Security

- Coordinate audit appends, checkpoints, verification, exports and retention through the database, preserving the existing signed audit formats.

### Migration

- Stop 0.5 writers, back up the database and signing keys, and run Goose migrations through one 0.6 instance before starting peers. Set audit.multi_instance on every MySQL/PostgreSQL instance and distribute the same persistent signing keyring; SQLite remains single-instance. Mixed-version writers and lossless downgrade after 0.6 retention are not supported.
- Legacy checkpoints with ambiguous ephemeral key IDs require the original keyring or explicit development-only audit.allow_legacy_ephemeral_recovery consent; configured persistent signing cannot silently downgrade when its configuration is omitted.
- Startup audit verification/initialization has a separate configurable deadline (30 minutes by default, up to 24 hours); ordinary audit operations remain bounded to 30 seconds.

## 0.5.0 - 2026-09-06

### Changed

- Introduce maintained GORM v2 database adapters and a pgx-backed PostgreSQL connection layer, preserving verified TLS, explicit pool limits, Goose schema ownership and secret-free connection errors.
- Establish shared persistence and security regression contracts for SQLite, strict MySQL and PostgreSQL before modernizing the ORM, preserving explicit Goose schema management and existing database compatibility.
- Move all application persistence to GORM v2 and the maintained pgx-backed PostgreSQL driver; remove legacy GORM and lib/pq without changing the Goose-managed schema.
- Preserve explicit zero-value writes, nullable timestamp hooks, missing-record errors, ownership predicates and manually maintained associations; isolate reused query statements and propagate aggregate errors.

### Fixed

- Restore default server startup after adding administrative CLI commands; bare invocation and explicit `serve` now start the configured servers while version, migration, secret and audit commands remain distinct.
- Reject permanent database trust/configuration errors immediately instead of delaying startup through transient connection retries; connection errors remain free of credentials and connection strings.
- Validate configured PostgreSQL CA files before opening the database, so unreadable or malformed trust files fail immediately with a redacted configuration error instead of transient connection retries.

### Security

- Preserve security transaction rollback on outbox failure and harden panic/commit handling; retain unscoped-write protection with explicit join-table inserts and scoped mail-lock updates.
- Extend the shared database contract with nested transactions, cancellation, failed PAT/reviewer/account changes, ciphertext-write failure, secret-rotation rollback, summary ownership and unchanged campaign-association checks.

### Migration

- Verify direct upgrade from real v0.4-created SQLite, MySQL and PostgreSQL data, including unchanged startup schema/data, encrypted credential bytes, authorization, signed audit records, normal writes and restart read-back.
- Existing v0.4 integration settings may contain plaintext left by automatic association saves. GORM v2 campaign persistence prevents new association rewrites; inspect existing data with `secrets status` and explicitly repair it with `secrets migrate` after backing up and configuring the key provider. Startup preserves existing bytes and does not silently rotate secrets.

## 0.4.0 - 2026-09-06

### Fixed

- Make generated vendor-bundle file permissions deterministic across Windows and Linux builds instead of inheriting mixed vendored source permissions.

### Security

- Constrain post-login navigation to known administrative pages and numeric campaign details; reject external, ambiguous, unknown and action-triggering return destinations without changing simulation redirects.
- Require a read-only, zero-open-CodeQL-alert baseline from current protected default-branch analyses before release preparation and native publication; fail closed on unavailable, stale or inconsistent security evidence.
- Replace incomplete pattern-based rewriting in administrative date and spellcheck helpers with strict date parsing and literal query-name matching, preserving encoded spellcheck values.
- Restrict template-name autocomplete to the intended alphabetic character range.
- Use an explicit parameterized recipient identity query, including empty fields, with SQLite, strict MySQL and PostgreSQL regression coverage for input and ownership isolation.
- Require trusted TLS certificates and explicit HTTP(S) URL validation for administrative site imports; retain connect-time egress restrictions on every redirect and bound request time, headers, and decompressed page size.
- Encode imported URL metadata as HTML attributes and return bounded, secret-free upstream errors without altering authorized simulation page content.

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
