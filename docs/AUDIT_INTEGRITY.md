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
# Multi-instance audit coordination (0.6)

For multiple audit-writing processes, use MySQL or PostgreSQL and set
`audit.multi_instance` to `true` on every process. Provision the same persistent
audit signing keyring on all instances. The database stores public-key
fingerprints and rejects mismatched or missing keys, including a process that
omits multi-instance mode after it has been enabled. Private keys remain in
the existing configuration/key storage. SQLite remains single-instance only.

Stop all 0.5 writers before upgrading. Back up the database and keyrings, apply
Goose migrations through one 0.6 instance, verify the audit, then start the other
0.6 instances. Mixed 0.5/0.6 writers and concurrent migration runners are not
supported. Existing event hashes and signed checkpoints are not re-signed.

All audit writes, checkpoints, verification, exports and retention take the
chain-head row lock in a transaction. This prioritizes consistency over audit
throughput: a large verification/export can delay writers. Transactions have a
30-second deadline and at most six attempts, with bounded backoff only for typed
deadlock, serialization and lock-contention errors. Integrity errors and unknown
commit outcomes are not blindly retried. Outbox retries use durable receipts.

Retention deletes only a contiguous prefix and commits its signed checkpoint
and retention boundary atomically. The last sequence/hash survives full event
retention. Delivery receipts intentionally survive retention and contain only
numeric outbox/event/sequence identifiers; do not purge them while delayed
delivery is possible. They cannot reconstruct already-retired pre-upgrade events.

For signing-key rotation, stop writers, distribute the expanded keyring to all
instances, switch the active key, and restart. Keep historical keys for signature
verification. Once registered, a key ID must never identify different key material.
