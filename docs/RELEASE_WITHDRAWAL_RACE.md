# Stale execution versus invalid publication

Recovery run35238957040 verified the original v0.12 native publication while an
external merge advanced protected main. The final execution check failed, but
generic provenance catches treated that as invalid artifact provenance and
withdrew release390565224. Its eight artifact identities/digests remained intact.

Execution authorization failures now have a distinct error type. Every native,
recovery and fallback catch propagates that failure instead of selecting another
publication path or withdrawing anything. This covers changed main/protection,
failed or unavailable execution checks and baseline/API verification errors.
Every publication-guard withdrawal target and retry revalidates the immutable
execution SHA immediately before PATCH, outside the ambiguous-write catch.

This is not an atomic branch-and-release transaction: GitHub release PATCH does
not support a branch-SHA precondition. An external actor can still move main
between the final read and PATCH. Shared workflow serialization is preserved;
maintainers should avoid concurrent manual main mutations during publication.
Invalid provenance with stable, authorized execution still follows the existing
fail-closed withdrawal and confirmation path; signatures, asset checks and
required CI/reviews are not weakened.

This change itself does not publish, restore, delete or rebuild any release.
The owner separately approved restoring the existing v0.12 release only after
fresh verification, without replacing its tag or artifact bytes. Restoration
must record the new publication timestamp honestly, retaining the original
native provenance and historical proof rather than claiming a new native build.

## Review timestamp precision

PR86 exposed an independent review-integration boundary: authentic completion
was reported at18:47:07.039349Z, while the GitHub summary updated_at was
18:47:07Z. Only the summary's self-consistency comparison now uses the precision
actually present in that API timestamp. The PR87 follow-up found that checking
only for a decimal point incorrectly treated all fractions as milliseconds.
Comparisons now retain nanoseconds and use the actual zero-to-nine fractional
digit count for summary self-consistency only. Review/finding order, identities,
full head SHA, content provenance and pre-merge completion remain unchanged and
strict. A preceding-second summary or completion after merge still fails.

## Tagless withdrawal follow-up

PR87 review also identified the separate missing-tag guard as a withdrawal path
not covered by the first fix. Its workflow now passes the immutable execution
SHA explicitly. Before each target and each ambiguous PATCH retry, the guard
synchronously invokes the same trusted publication guard's execution-only mode.
Any failed execution check stops outside the PATCH retry catch, without an
absent-tag output authorizing later recovery. Tests cover multiple targets,
missing SHA, stale authorization and retries; genuine tagless publications
still follow the existing fail-closed withdrawal policy. The residual GitHub
read/PATCH race described above remains: this does not create an atomic CAS.
