# One-time v0.7.1 repair authorization

The maintainer explicitly authorized a narrowly scoped, reviewed release-repair
policy on 2026-09-14. PR56 may repair historical release provenance while the
pending-publication engineering freeze otherwise remains active.

The manual `releaseRepair: true` merge option is limited to PR56 in this public
repository, its named branch and immutable base
`a2851cd96e327175b3d45b8949ad64492b52b21f`. The caller must pin the complete
reviewed head SHA. The API revalidates allowed changed paths, VERSION 0.7.1 and
the unchanged normalization hold before and after all existing merge checks.
Another PR, changed base, unauthorized file change, hold removal or version bump
fails closed. The exception becomes unusable when protected main moves.

No workflow opts into this mode. A maintainer-approved local invocation of the
exact reviewed helper may perform the bootstrap merge only after authentic code
and security reviews, resolved threads, all required checks, zero open CodeQL
alerts, stable protected base and GitHub mergeability checks pass. It must not
use administrator bypass or native queued auto-merge. Privileged Actions
workflows continue to execute protected-main code only.

This authorization does not publish either draft, choose different artifacts,
move a tag, remove the normalization hold, or alter the publication verifier.
Those actions still require their own verified recovery prerequisites.
