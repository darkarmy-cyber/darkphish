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
than zero, an AES-256-GCM v2 envelope containing one credential. The envelope
includes its version, key ID, algorithm, nonce, and ciphertext. Values never
appear in campaign result payloads, audit metadata, normal exports, webhooks, or
application logs. Values longer than the configured maximum are reported as a
policy failure but are not retained. There is no bulk reveal, export, or replay
feature.

Reveal is an explicit POST action for one result. It requires campaign ownership,
the administrator-only `credentials:view` role permission, and—when a PAT is
used—the `credentials:view` scope. The UI remains masked until the operator
confirms the action. Every attempt is audited as `credential.view`.

Encrypted retention defaults to 24 hours and is capped at 30 days. Zero means
evaluate and immediately discard. The background worker clears expired
ciphertext while keeping policy findings. Database files, backups, binary logs,
snapshots, and storage replicas can retain historical encrypted pages; apply
equivalent retention to those systems and keep encryption keys protected.
