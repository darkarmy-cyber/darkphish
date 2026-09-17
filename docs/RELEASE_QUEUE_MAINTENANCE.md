# Protected release queue and historical maintenance

All seven protected mutation workflows retain the same global
`protected-main-mutation` lock and `cancel-in-progress: false`. Each declares
`queue: max`: at most one running and up to 100 pending operations. A full queue
rejects additional arrivals; this is not an unlimited or guaranteed-delivery
queue. Waiting order is determined when a run enters the queue, not dispatch
time. See [GitHub concurrency documentation](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/control-workflow-concurrency).

Do not split publisher, recovery and merge locks. Queued operations retain their
source/main, current checks, review, publication identity and asset checks.
Stale work must fail closed or return a verified no-op; no run may replace an
existing release's artifacts. Old runs created before this change still carry
their original queue policy. Observe them draining before judging the new policy;
do not cancel unrelated runs or bypass a failed check.

## Strict validator compatibility

Upstream actionlint v1.7.12 does not understand the new queue key. The pinned,
unmodified binary still checks every workflow, including local action metadata,
expressions and shell scripts. `scripts/workflow-lint.mjs` first requires the exact
literal three-line concurrency policy in each protected workflow, then removes
only the validated queue line from its in-memory lint input. It retains original
line numbers and filenames; no source file or linter diagnostic is rewritten or
filtered. Other syntax and policies are not silently accepted. The required CI
job includes integration tests against the real binary, with malformed workflow
and expression fixtures that must fail. Once upstream supports this key, remove
the compatibility projection in a reviewed change while retaining policy tests.

## Historical operations

The completed v0.7.1 resumption, title repair and historical metadata
reconciliation no longer run on schedules or CI completion. They remain manual
operations on protected `main`, with explicit confirmation inputs and the shared
queue. A queued stale SHA cannot authorize a write. The confirmation strings are
not credentials or replacements for permissions and review.

- `release-resume.yml`: `resume-original-071`; existing exact-release,
  authorization and provenance constraints remain in force.
- `release-title-repair.yml`: `repair-071-title`; exact green protected main is
  checked before the existing fixed-release title checks and update.
- `release-reconcile.yml`: `reconcile-public-metadata`; only public releases are
  considered. Drafts are logged and skipped before asset access or mutation.
  Public-release identity/provenance failures still fail the run. Detached draft
  tag repair is no longer part of this workflow.

Retained drafts 388809146 (v0.8.0) and 388031151 (v0.7.1) are not deleted,
published, renamed or adopted by this change. Draft cleanup is a separate,
explicitly authorized operation requiring a verified backup and preservation of
the public release IDs, tags and assets. This change does not grant that authority.

Automatic native publication, recovery, reviewed merge and conservative
Dependabot merge remain enabled with all previous gates. To diagnose a future
delay, inspect running/pending members, their immutable source and actual job
results; a skipped or canceled run is not publication success. Never dispatch
parallel publishers to work around queue saturation.
