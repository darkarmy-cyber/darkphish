# v0.22.2 publication recovery

The protected release PR #162 merged as
`743633afd812980852480cf746a168c75814cde5`. Native run 36698961221
attempt 2 checked out this source using the mutable `main` reference, but its
workflow execution commit was `46f85af06b049487ed363afbee80eab68f59d4de`.
Its signatures therefore do not satisfy the updater's exact-source policy.
Verifying the publication receipt with the embedded trusted root, source digest
and signer digest set to the release commit reproduces the signer mismatch.

Recovery run 36717602983 withdrew release 400025048 to draft, preserving the
immutable tag and its assets. Fresh native run 36717602969, attempt 2, executed
at the correct release commit but failed only in metadata discovery with
`existing release state is not a known automation artifact`; its build and
publication jobs were skipped. The draft remains retrievable by its ID.

The recovery policy recognizes only that exact completed metadata failure:
repository, source, workflow/run/attempt IDs, actor, timestamps, all five unique
job IDs and their exact outcomes, and the metadata step sequence are pinned.
It does not recognize the stale successful publication as trusted evidence.
This follows the existing audited metadata-failure recovery mechanism.

The exception only supplies historical recovery eligibility. The existing
source CI/CodeQL chronology, generated release PR and maintainer attestation,
protected execution commit, source ancestry, tag immutability, full rebuild,
verification, smoke tests, SBOM, checksums, fresh attestation and publication
checks remain mandatory. Historical draft assets must not be reused as rebuilt
artifacts or deleted. No updater trust policy is changed.

Do not rerun native publication from an older execution commit. Pinning its
metadata checkout to `github.sha` is a separate follow-up after this recovery;
this repair only changes the recovery workflow and its documented eligibility.

After protected merge, run CI on current main to create a fresh recovery run.
Do not rerun the old recovery execution. Verify every published asset's digest
and signature, the recovery publication interval, the receipt's original source,
the unchanged tag, and the latest release identity before declaring completion.
