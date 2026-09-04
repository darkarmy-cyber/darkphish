# Architecture

Darkphish remains a single deployable Go binary with two HTTP trust boundaries:

1. The administrative server serves authenticated HTML and `/api/` routes. It
   owns sessions, CSRF enforcement, RBAC, explicit CORS, request limits, security
   headers, request IDs, audit records, and health endpoints.
2. The simulation server renders administrator-authored landing pages and tracks
   authorized campaign interactions. It intentionally does not inherit the
   restrictive administrative CSP because user templates are executable content.

Both servers use the models/store layer and background worker. SQLite remains the
default. MySQL/MariaDB remains supported by the existing migration set. PostgreSQL
is a future workstream and will require its own migration and campaign test matrix.

## Security boundaries

- Browser authentication is an encrypted, authenticated cookie session. Mutating
  session requests remain CSRF protected.
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
  permission-checked, audited POST action. Normal APIs, exports, webhooks, and
  logs never receive the raw value. Darkphish does not provide credential replay.

## Audit and observability

Administrative requests receive an application-generated `X-Request-ID`.
Security-sensitive API mutations, campaign result access, login, logout, and
impersonation emit persistent indexed events and matching JSON-line records with
timestamp, actor, action, target, result, request ID, source IP, and
authentication method. The audit interface does not accept request bodies,
passwords, tokens, or secret values. The administrator viewer uses server-side
filters and pagination; retention cleanup is configured independently.

`/healthz` reports process liveness. `/readyz` verifies database connectivity.
Metrics and tracing are intentionally deferred, but request correlation and
bounded server behavior provide their foundation.

## Evolution direction

Business logic should move gradually from handlers and GORM models into focused
`internal/` packages. The next persistence milestone is repository interfaces
and migration tests before any GORM v2 or PostgreSQL change. Darkphish should not
be split into microservices without measured operational benefit.
