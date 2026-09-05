# Architecture

Darkphish remains a single deployable Go binary with two HTTP trust boundaries:

1. The administrative server serves authenticated HTML and `/api/` routes. It
   owns sessions, CSRF enforcement, RBAC, explicit CORS, request limits, security
   headers, request IDs, audit records, and health endpoints.
2. The simulation server renders administrator-authored landing pages and tracks
   authorized campaign interactions. It intentionally does not inherit the
   restrictive administrative CSP because user templates are executable content.

Both servers use the models/store layer and background worker. SQLite remains the
default; MySQL/MariaDB and PostgreSQL have database-specific migration histories
and hosted integration gates.

## Security boundaries

- Browser authentication is an encrypted, authenticated cookie session. Mutating
  session requests remain CSRF protected. Credential reveal adds an independent,
  five-minute privileged session: a password proof is verified through the
  `Reauthenticator` service and the resulting server-side grant is bound to an
  opaque current-session identifier. Passwords and reusable privileged bearer
  values are never stored in the browser.
- External API authentication is an explicit bearer credential. A request with a
  malformed Authorization header never falls back to a browser session.
- Account lock and required-password-change state are checked by one policy for
  both mechanisms.
- Integration model secrets are write-only DTO fields. The secret-store interface
  separates persistence from AES-256-GCM protection and is designed for future
  Vault/KMS implementations.
- Outbound imports use a connect-time restricted dialer. It validates resolved
  addresses to resist DNS rebinding and denies local, private, metadata,
  multicast, unspecified, and reserved ranges unless explicitly allowlisted.
- Credential handling is campaign-scoped and disabled by default. Policy-only
  mode persists irreversible measurements; encrypted review stores a bounded-
  retention AES-256-GCM envelope and exposes plaintext only through a dedicated,
  permission-checked, fresh-authenticated, audited POST action. A System
  Administrator may review across campaigns. A Security Reviewer needs both the
  narrow `credentials:view` permission and a live campaign assignment. Ownership
  alone does not grant reveal. Normal APIs, exports, webhooks, and
  logs never receive the raw value. Darkphish does not provide credential replay.

Authorization is centralized in account-state, role-permission, campaign-access,
reviewer-assignment, PAT-scope, and privileged-session helpers. Controllers parse
and format requests but do not independently recreate that decision tree.

## Persistence boundaries

New security-sensitive code uses focused `CampaignRepository`,
`CredentialRepository`, `TokenRepository`, `ReviewerRepository`, and
`AuditRepository` interfaces. Credential review, reviewer, token, secret rotation,
and audit operations have explicit service boundaries while inherited CRUD remains
on GORM v1.

Campaign creation commits the campaign, credential policy, initial event, results,
and mail queue in one transaction. Credential evaluation commits the finding and
optional ciphertext—or destruction of a superseded value—together. Reviewer
changes, PAT mutations, security-role updates, and each secret re-encryption commit
their business state with an audit-outbox row. After commit the
outbox is delivered to the hash chain; a failure remains visible and retryable at
startup and on the next protected operation. This gives business atomicity without
placing remote logging inside a database transaction.

## Audit and observability

Administrative requests receive an application-generated `X-Request-ID`.
Security-sensitive API mutations, campaign result access, login, logout, and
impersonation emit persistent indexed events and matching JSON-line records with
timestamp, actor, action, target, result, request ID, source IP, and
authentication method. Events form one ordered instance chain using deterministic
canonical JSON, durable-delivery IDs, sequence numbers, previous hashes, and
SHA-256 event hashes.
Purpose-specific Ed25519 keys sign periodic checkpoints and retention anchors.
Signed export manifests bind record range, file digest, application version,
commit, and checkpoint reference. The audit interface does not accept request bodies,
passwords, tokens, or secret values. The administrator viewer uses server-side
filters and pagination; retention cleanup is configured independently.

## Key-management boundary

The provider-neutral routing store writes v3 AES-256-GCM envelopes. A random data
encryption key protects each value; a configured provider wraps only that key.
The local versioned keyring remains supported, and Vault Transit is the first
external provider. V1/v2 ciphertext stays readable while its local key remains
configured. Provider outages and unknown key references fail closed; there is no
implicit fallback to a different provider.

`/healthz` reports process liveness. `/readyz` verifies database connectivity.
Metrics and tracing are intentionally deferred, but request correlation and
bounded server behavior provide their foundation.

## Dependency choices for 0.3

PostgreSQL uses maintained MIT-licensed `github.com/lib/pq` through GORM v1's
official PostgreSQL dialect. This adds no application SDK graph and avoids an ORM
rewrite in a security release. Vault Transit uses the standard HTTP/TLS packages
instead of a large Vault SDK; Ed25519, AES-GCM, SHA-256, and randomness use the Go
standard library. The pinned modules are covered by Dependabot, govulncheck,
license/SBOM generation, and the Go 1.27 build gate.

## Evolution direction

Business logic should continue moving gradually from handlers and GORM models into
focused services. PostgreSQL uses the safest incremental GORM v1/lib/pq path for
0.3; replacing the inherited ORM is technical debt, not a release-time rewrite.
Multi-instance audit serialization will need database-native coordination before
horizontal write scaling. Darkphish should not be split into microservices without
measured operational benefit.
