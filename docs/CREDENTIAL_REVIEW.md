# Credential review controls

Darkphish campaigns default to `disabled`: submitted password values are not
evaluated or retained. A landing page must also explicitly allow password fields
to be submitted before either review mode can observe them.

`policy_only` evaluates length bounds, configurable minimum uppercase,
lowercase, digit, and symbol counts, disallowed patterns, and a local strength
score. All class minimums may be zero, so an organization can use length-only
policy. It persists only counts, pass/fail state, failure identifiers, and the
evaluation time. These fields are insufficient to reconstruct the submitted
value.

`encrypted_review` stores the same policy result and, when retention is greater
than zero, an AES-256-GCM envelope containing one credential. Local v2 envelopes
include a key ID; provider-neutral v3 envelopes include provider, key reference,
wrapped data key, algorithm, nonce, and ciphertext. Values never
appear in campaign result payloads, audit metadata, normal exports, webhooks, or
application logs. Values longer than the configured maximum are reported as a
policy failure but are not retained; a later non-retainable submission also
destroys any older ciphertext for that result. There is no bulk reveal, export,
or replay feature.

Reveal is an explicit POST action for one result. It requires an active account,
campaign access, the `credentials:view` role permission, and—when a PAT is used—
the `credentials:view` scope. A System Administrator is authorized across
campaigns. A Security Reviewer is authorized only while an explicit assignment
to that campaign is active. A campaign owner without the credential permission is
not authorized.

Browser sessions also require the current account password again. A successful
proof creates a five-minute server-side privileged grant bound to the current
browser session. A different session, an expired grant, account lock, password
change, logout, or impersonation cannot reuse it. PAT access does not use browser
reauthentication, but still requires the scope, current user permission, account
state, and campaign assignment. Reauthentication success/failure and every reveal
attempt are audited. Sensitive endpoints are limited per user and source address.

The UI remains masked until an operator explicitly reveals one result. Responses
are marked `Cache-Control: no-store`; the transient value is not put in local or
session storage and is removed from the dialog when it closes.

Encrypted retention defaults to 24 hours and is capped at 30 days. Zero means
evaluate and immediately discard. The background worker clears expired
ciphertext while keeping policy findings. Database files, backups, binary logs,
snapshots, and storage replicas can retain historical encrypted pages; apply
equivalent retention to those systems and keep encryption keys protected.
Expiration destroys the stored ciphertext. Reauthentication cannot restore an
expired value; the irreversible policy finding remains available.

Campaign owners and System Administrators can assign or remove active Security
Reviewer accounts from the campaign-results screen and can set an optional expiry.
Assignments do not transfer ownership or grant campaign mutation, user-management,
or system-management authority. Assignment, removal, and natural expiry events are
written through the durable audit outbox and never contain credential values.
