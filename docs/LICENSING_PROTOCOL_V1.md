# Darkphish Licensing Protocol v1

Status: **development contract for Darkphish Community 0.11.x**

This document defines the wire and trust contract between an official Darkphish
Community installation and the Darkphish licensing service. The first server
implementation is expected to be the fsociety WordPress licensing plugin; the
protocol is intentionally independent of WordPress.

## Goals

- Community remains free but requires registration and activation.
- Community entitlements are server-issued rather than hard-coded as the source
  of truth.
- An installation can continue through temporary licensing-service outages.
- License signatures are verifiable locally without embedding a signing secret
  in Darkphish.
- Expiration or revocation must never make customer data inaccessible.
- The protocol must be reusable by later editions without changing the v1 trust
  model.

## Community policy baseline

The initial Community policy is:

- price: free
- managed users: 100 unique normalized recipient e-mail addresses per instance
- active campaigns: 1 per instance
- completed campaign history: unlimited
- suggested license validity: 365 days
- suggested activation lease: 30 days
- client refresh interval: 1 hour by default (configurable from 60 to 3600 seconds)
- suggested offline grace: 30 days

The policy values above are defaults for issuance. Runtime enforcement consumes
values from a valid signed lease.

## Trust model

Darkphish contains trusted **public** signing keys only. The licensing service
holds the corresponding Ed25519 private online signing key outside the public
web root and outside ordinary WordPress options/tables.

Production deployments SHOULD use a key hierarchy in which an offline root key
approves rotating online signing keys. v1 lease verification is keyed by
`key_id`, allowing online key rotation without changing the lease schema.

Private signing material MUST NOT be returned by any API, written to logs, or
stored in the Darkphish application.

## Installation identity

At first activation Darkphish generates a cryptographically random installation
identifier. Hardware identifiers, MAC addresses, CPU serial numbers, disk
serials and similar fingerprints are not part of v1.

Recommended representation: UUIDv4 or 128 bits of cryptographically random data
encoded as a UUID.

One Community license initially permits one active installation. Resetting an
activation is a licensing-server administrative operation and is audit logged.

## Activation flow

1. The user obtains a free Community license after e-mail verification.
2. Darkphish generates and persists an `installation_id` locally.
3. Darkphish sends an HTTPS activation request containing the license key,
   installation ID and running Darkphish version.
4. The licensing server validates the license, policy and installation count.
5. The server returns a signed activation lease.
6. Darkphish verifies the Ed25519 signature locally before trusting any field.
7. The verified lease is persisted locally and becomes the entitlement source.

Suggested endpoint:

`POST /wp-json/darkphish-license/v1/activate`

Example request:

```json
{
  "license_key": "DP-COM-XXXX-XXXX-XXXX",
  "installation_id": "90b594f8-d605-4b68-af68-e086749fc4bd",
  "product_version": "0.11.0"
}
```

The service SHOULD return a generic authentication failure for unknown,
malformed or revoked keys rather than exposing license-account metadata.

## Refresh flow

A valid installation periodically refreshes its lease over HTTPS. Refresh is
not required for every application request.

Suggested endpoint:

`POST /wp-json/darkphish-license/v1/refresh`

The request identifies the license and installation using an activation
credential established during activation. The raw public license key SHOULD NOT
be repeatedly transmitted if a narrower refresh credential can be used.

The service can use refreshes to record minimal operational metadata such as
product version and last-seen time. Optional telemetry beyond licensing status
must be documented separately and must not be silently added to this protocol.

## Signed lease envelope

The HTTP response contains an envelope. `payload` is base64url without padding
of the exact UTF-8 JSON bytes that were signed. `signature` is base64url without
padding of the Ed25519 signature over those exact payload bytes.

```json
{
  "key_id": "DP-COM-2026-01",
  "algorithm": "Ed25519",
  "payload": "eyJzY2hlbWEiOi4uLn0",
  "signature": "base64url-ed25519-signature"
}
```

Signing the encoded payload bytes rather than re-serialized JSON avoids
cross-language canonical-JSON ambiguity.

## Lease payload

After signature verification the decoded payload has this logical form:

```json
{
  "schema": "darkphish-license-lease/v1",
  "product": "darkphish",
  "edition": "community",
  "license_id": "DP-COM-000381",
  "installation_id": "90b594f8-d605-4b68-af68-e086749fc4bd",
  "issued_at": 1789473600,
  "not_before": 1789473600,
  "expires_at": 1792065600,
  "grace_until": 1794657600,
  "entitlements": {
    "managed_users": 100,
    "active_campaigns": 1
  }
}
```

Times are UTC Unix seconds. Servers and clients must tolerate reasonable clock
skew around `not_before`; the exact skew budget is an implementation setting,
not a field controlled by the lease.

## Runtime states

A client maps a verified lease to one of these states:

- `active`: current time is within `not_before..expires_at`
- `grace`: lease has expired but current time is not later than `grace_until`
- `expired`: current time is later than `grace_until`
- `invalid`: signature, schema, product, installation binding, or payload
  validation failed
- `missing`: no lease has been activated

A server-side revocation prevents further lease issuance. An already issued
lease remains usable until its signed grace deadline; the client preserves the
last verified lease when the service rejects a refresh. Protocol v1 has no
signed revocation object, and an HTTP error is not proof that a previously
verified lease must be erased. With the default issuance policy, offline
revocation latency is at most 60 days after the last lease was issued, bounded
also by the license's expiry. Installation resets have the same offline limit.

## Safe degraded mode

When state is `expired`, `invalid` or `missing`, Darkphish must preserve access
to customer data and allow risk-reducing operations.

Allowed:

- authenticate administrators
- view campaigns and results
- export data
- complete an existing campaign
- delete data
- enter/replace a license and activate

Blocked:

- create or start a campaign
- schedule a new campaign
- add a new managed user
- expand a group beyond the licensed managed-user entitlement

`grace` retains normal licensed operation while prominently warning the
administrator that renewal/refresh is required.

## Managed-user definition

For Community enforcement one managed user is one unique normalized recipient
e-mail address present in the instance. Group membership is not counted.

The same address in multiple groups is one managed user. Comparison is based on
trimmed, case-folded e-mail text. Enforcement must calculate the projected
unique count transactionally for both group creation and group update.

Existing installations upgraded while already above the limit are not modified
or truncated. They may reduce/delete data, but cannot add a new distinct
managed user until usage is within entitlement.

## Active-campaign definition

Community permits one active campaign. A campaign counts as active until it is
explicitly completed. Queued, created, in-progress and emails-sent campaigns
therefore consume the active-campaign entitlement. Completed campaign history
is not limited.

The limit is enforced in the backend model/service layer so GUI, API and PAT
clients cannot bypass it.

## Server persistence model

The licensing service should use dedicated tables rather than WordPress
`wp_options` for operational records. Suggested logical entities:

- license policies (edition/version policy and default validity)
- licenses (issued entitlement snapshot and account identity)
- activations (installation binding, status, version, last seen)
- signing-key metadata (key ID, public key, fingerprint, lifecycle state)
- append-only license events/audit trail

A policy change does not silently mutate already issued entitlement snapshots.
Renewal or explicit administrative reissue can move a license to a newer policy.

## Public Community request API

The public generator on `darkphish.sk` is a static frontend only. It does not
contain a signing secret or privileged API credential. It calls the licensing
service over HTTPS.

The public request flow must include server-side rate limiting, anti-automation
protection, e-mail verification and acceptance of the Community license terms.
Marketing consent, if offered, is separate and optional.

CORS may restrict browser origins to official Darkphish sites, but CORS is not
authentication and must not be treated as the security boundary.

## Data minimization

Required server records should be limited to information needed for licensing,
security and support: license ID, verified e-mail, optional company/country,
installation ID, product version, activation/refresh timestamps, state and
policy snapshot. Long-term IP-address retention is not required by the v1
protocol.

## Security requirements

- HTTPS is mandatory for activation and refresh.
- Ed25519 signatures are verified before payload parsing is trusted.
- Unknown `key_id` and algorithms fail closed.
- Lease payload is bound to `product`, `edition` and `installation_id`.
- Server activation and administrative changes are rate limited and audited.
- License keys are treated as credentials and are never written to normal logs.
- Private signing keys are outside the WordPress database and public web root.
- Signing-key rotation and emergency revocation procedures must be documented
  before production launch.

## Versioning

Protocol-breaking changes require a new schema value such as
`darkphish-license-lease/v2` and a new REST namespace or negotiated protocol
version. Adding server-side policy records does not by itself change this wire
protocol.

## Runtime configuration (v0.11)

Normal server startup always installs a licensing manager before starting the
HTTP servers and campaign worker. Missing service configuration starts in
unactivated mode: administrators retain access to data and settings, while
new managed users and campaign delivery are blocked. Offline audit and migration
commands retain their compatibility behavior.

Configure both the approved HTTPS API URL and the public keyring in `config.json`:

```json
{
  "license": {
    "service_url": "https://YOUR-LICENSE-SERVICE/wp-json/darkphish-license/v1",
    "keyring_file": "/etc/darkphish/license-public-keys.json",
    "state_path": "/var/lib/darkphish/license-state.json",
    "refresh_interval_seconds": 3600
  }
}
```

The URL above is a placeholder, not a deployed service. The release must not
ship an invented production key or a development signing key. The public
keyring uses `darkphish-license-keyring/v1` and maps key IDs to standard Base64
Ed25519 public keys. Never place the private signing key in Darkphish.
Relative configured paths resolve from the configuration file directory.
The default state file lives in the configured bootstrap directory, or beside
the configuration file when no bootstrap directory is configured. Keep it on
persistent private storage and include it in secured backups.

Refresh runs at startup for an existing activation and every configured interval
(60–3600 seconds; default one hour). Refresh and activation exchanges are
serialized so rotated tokens cannot overwrite a newer exchange. Shutdown
cancels an in-flight refresh. Service failure preserves the last signed lease;
its expiry and grace timestamps continue to govern access. Redirects are
rejected before forwarding any credential.

The Settings → Licensing page shows state, installation, expiry and usage, and
supports activation and manual refresh. It is restricted to administrators.
Activation remains unavailable until both service configuration and the public
keyring are installed.

A v0.11 migration adds only the singleton `license_coordination` table. Group
changes and campaign creation lock that row inside the same transaction that
reads usage and commits the change. PostgreSQL/MySQL use READ COMMITTED;
SQLite acquires its writer lock before usage reads. Existing schema and
customer data are retained. Competing transactions are tested on every backend.
Queued campaigns and delivery retries recheck current license state before
sending. A denied queued batch remains retryable after reactivation; access to
results and explicit campaign completion remain available.
