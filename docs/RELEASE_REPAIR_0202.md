# One-time v0.20.2 review-provenance recovery

Generated release PR #119 was merged at
`4be0bc0d1a6a2f158da5aaed996dcd87bab3bd80` before its required maintainer
attestation existed. Native publication and recovery correctly failed closed;
no `v0.20.2` tag or GitHub Release was created.

PR #120 authorizes one recovery path for that immutable generated-only merge.
The verifier pins the repository, PR number, release branch, head, base, merge
commit, merge timestamp, Actions creator, repository-owner merger, and every
changed file's path, operation, blob SHA and line counts. It also requires zero
unresolved review threads and re-reads the final PR snapshot. Any mismatch
blocks publication.

The repair merge itself is pinned to PR #120, branch
`fix/recover-v0.20.2-review-provenance`, base
`4be0bc0d1a6a2f158da5aaed996dcd87bab3bd80`, VERSION `0.20.2`, an eleven-file
allowlist, and the continued absence of both the tag and release. The same
identity and exact file set are the only case allowed to stage the `0.20.3`
changelog fragment before `v0.20.2` exists. It must pass the ordinary exact-head
code and explicit security reviews, required checks, CodeQL baseline,
resolved-thread and protected-merge gates. It does not permit tag movement,
artifact substitution, a different release PR, or any future release without
its normal pre-merge maintainer attestation.

The one-time boundary recognizes only Native release run `36353391121` (run
352) and its exact metadata-stage failure, job manifest and step manifest. With
no tag or draft, recovery may select only the immutable PR #119 merge after
re-running the complete audited maintainer-review verifier. The immediate
protected-main squash of PR #120 is the sole post-merge push allowed to carry
the pending `0.20.3` fragment while `v0.20.2` remains unpublished.

After protected merge, the recovery workflow rebuilds the immutable v0.20.2
source, produces the complete native archive set, checksum manifest, SPDX SBOM
and publication receipt, attests them, creates the immutable tag and release,
and verifies the public result.
