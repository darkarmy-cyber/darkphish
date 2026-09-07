# Darkphish 0.7: bounded audit history reads

Baseline: `c8ac718249d3f9a5796a2345759bd8879c99d4d1` (released Darkphish 0.6).

Audit verification and legacy initialization previously materialized all events
and checkpoints. Verification runs during startup, explicit checkpoint creation,
retention, and signed export. This workstream bounds those database result sets
to 256 rows each, independent of history length.

## Invariants

- The same 0.6 database chain lock covers every page of an operation. No snapshot
  is assembled from separately committed transactions, and writers/retention
  cannot interleave with the traversal. SQLite remains single-instance only.
- Events use `(chain_sequence, id)` keysets; checkpoints use `(last_sequence,
  id)`. Legacy initialization uses `id` because sequence values are not assigned
  yet. Initial queries have no lower cursor, so invalid zero/negative values are
  visible. Queries have a fixed LIMIT and no OFFSET; cursor values are bound SQL
  parameters. A page is fully fetched before additional transaction queries.
- Every event hash, sequence link, retention anchor, final head, checkpoint
  signature and retained checkpoint linkage is still verified. Traversal is not
  a sampling check. No stored hashes, signatures, canonical formats or schema
  change. Startup's existing lost-ephemeral-key development exception is unchanged.
- Failure on a later page aborts the whole transaction. Legacy hash/receipt
  backfills roll back together; typed retry creates new iterators and report/output
  state. No partial export is returned after verification or commit failure.
- Signed JSON export bytes match the former `json.MarshalIndent` array encoding,
  including empty arrays and the trailing newline. The export constructor no
  longer holds a full row slice plus a second event slice alongside the payload.

## Bounds and remaining limitations

Verification, legacy backfill, and retention keep at most a bounded page of
events/checkpoints plus individual records. Memory still depends on the size of
an individual stored record; this is a row-count bound, not an arbitrary byte cap.
`BuildAuditExport` still returns `[]byte`, and `VerifyAuditExport` still accepts
and decodes a complete payload. Those public/CLI interfaces are **not** claimed
to have constant total memory. Signing identity metadata is small in normal
operation but is not covered by the event/checkpoint page bound.

Full verification remains O(history) and serializes against writers. Checkpoint
linkage still performs one event lookup per retained checkpoint. Ordinary
operations retain the shared 30-second deadline; startup retains its independently
configurable deadline. This release does not add whole-platform HA, online
mixed-version upgrades, automatic key rotation, receipt pruning or new capture,
replay, bypass or evasion capabilities.

## Verification

Deterministic runtime query guards check LIMIT, absence of OFFSET, and returned
row counts rather than relying on noisy process RSS thresholds. SQLite tests
span multiple event/checkpoint pages, compare exact old export encoding, corrupt
first/boundary/last records, test later-page errors and retry, and prove rollback
of legacy hash and receipt backfill. Existing tamper/retention/security tests stay
in force. MySQL/PostgreSQL six-process tests start with 266 retireable and 259
retained events/checkpoints, then run the original 240 delivery attempts and 12
outbox rows: 421 retained events, head sequence 687, retention anchor 266.
Native prepublication and downloaded-release smoke also cross a page boundary
with 270 denied loopback requests across three server processes, then restart,
verify the chain/export, and match every request ID to exactly one durable event.

There is no new Goose migration. Back up database and signing keys and perform a
coordinated upgrade; existing pre-0.6 migration and signing-key requirements still
apply. VERSION advances through the protected generated release PR to 0.7.0.
