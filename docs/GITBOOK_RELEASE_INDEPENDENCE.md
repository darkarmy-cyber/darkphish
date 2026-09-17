# GitBook and native audit test reliability

GitBook remains connected to the repository's documentation. Merged content
continues to synchronize; pull-request previews are informational, not release
approval. There is no GitBook disconnect, synthetic success status, force import,
branch-protection bypass, or change to documentation content in this repair.

The protected merge helper accepts `unstable` only when every latest check run
has completed successfully (or is neutral/skipped), all non-success commit
statuses are from the pinned GitBook bot and exact configured documentation
contexts, and none of those contexts is required by branch protection or rules.
Required application/security checks, both authenticated reviews, immutable
head/base validation, release attestation and GitHub's protected merge remain.
Unknown or forged statuses, unavailable APIs, conflicts, behind/blocked states,
and failed additional CI runs still stop merging. A later maintainer decision to
make GitBook required is respected automatically.

## MySQL startup failure

Run 35225777601 on release head 6d190b5715f41f4b6faaca8136815462b6c48346
failed at native audit process startup. The old harness discarded child output,
so the historical exit cause cannot be proved from that run's retained logs.
The separate passing PR CI does not invalidate that failure.

One independently identified harness defect was allocating ports by repeatedly
binding and closing before starting any server. The OS may return the same port
for multiple configurations. All allocations now stay bound simultaneously
until each child launch, including on restart. There remains an unavoidable
external bind race between release and child bind; such a failure is reported,
never retried or accepted as success. Bounded, credential-redacted child output
and exit/signal details make startup failures actionable. All concurrent startup
attempts finish before cleanup; the three processes must remain alive through
the concurrent request phase. The test still requires 270 distinct denied
requests, exact durable event coverage, a fourth verifier, restart and signed
export verification on both MySQL and PostgreSQL.

Release closure requires fresh CI on the corrected revision and both reviews.
The old run must remain recorded as failed, not dismissed or rerun to get green.
