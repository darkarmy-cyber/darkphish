# Audit integrity and exports

Darkphish 0.3 makes persisted audit modification detectable. Every event belongs
to the `instance` chain and has a monotonically increasing sequence, the previous
event hash, and a SHA-256 hash over deterministic canonical JSON. Canonical input
includes the event and durable-delivery IDs, UTC timestamp, bounded actor/action/target identifiers,
result, request metadata, authentication method, metadata JSON string, chain ID,
sequence, and previous hash. It never includes request bodies, submitted
passwords, PATs, integration secrets, ciphertext, or wrapped keys.

Writes are serialized within an instance and the database enforces chain indexes.
Reviewer, PAT, and user-security mutations first write a durable outbox row in the
same transaction as business state. Delivery failure is logged, retained, and
retried rather than silently discarded. A single writer instance per database is
required for 0.3; database-native multi-instance sequencing is deferred.

## Checkpoints and retention

At the configured interval (1,000 events by default), Darkphish signs a checkpoint
with the active Ed25519 audit key. It contains format version, chain and event
ranges, final hash, creation time, and signing key ID. Key IDs make rotation
explicit. Keep old verification keys configured while any checkpoint or export
signed by them must verify.

Retention deletes only a contiguous oldest prefix. Before deletion Darkphish signs
an exact anchor for the removed final sequence. Verification of the retained first
event requires that anchor, so deletion in the middle or an invented chain start
is detected. Retention policy is not a substitute for exporting records required
by legal or organizational policy.

## Operator commands

```sh
darkphish --config /etc/darkphish/config.json audit verify
darkphish --config /etc/darkphish/config.json audit export \
  --output darkphish-audit-2026-09-05.json
darkphish --config /etc/darkphish/config.json audit verify-export \
  darkphish-audit-2026-09-05.json \
  darkphish-audit-2026-09-05.manifest.json
```

The export command uses exclusive file creation and writes mode `0600`. Its signed
companion manifest binds format version, generation time, record count, first and
last event IDs, file SHA-256, checkpoint reference, application version, commit,
and signing key ID. Verification checks the signature, file digest, record count,
event range, complete exported hash chain, and referenced checkpoint linkage.

Verification fails closed for a modified event, missing middle event, changed
previous hash, changed sequence/order, bad checkpoint data or signature,
unsupported format version, modified export, missing checkpoint, or unknown key.
Run verification before and after backup/restore, signing-key rotation, retention,
and security investigations.
