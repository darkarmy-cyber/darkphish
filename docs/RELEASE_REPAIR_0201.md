# One-time v0.20.1 review-provenance recovery

Generated release PR #116 was merged at
`cafdcc08b9d968931cb1446891055e3256d0caec` before its required maintainer
attestation existed. Native publication and recovery correctly failed closed;
no `v0.20.1` tag or GitHub Release was created.

PR #117 authorizes one recovery path for that immutable generated-only merge.
The verifier pins the repository, PR number, release branch, head, base, merge
commit, merge timestamp, Actions creator, repository-owner merger, and every
changed file's path, operation, blob SHA and line counts. It also requires zero
unresolved review threads and re-reads the final PR snapshot. Any mismatch
blocks publication.

The repair merge itself is pinned to PR #117, branch
`fix/recover-v0.20.1-review-provenance`, base
`cafdcc08b9d968931cb1446891055e3256d0caec`, VERSION `0.20.1`, a six-file
allowlist, and the continued absence of both the tag and release. It must pass
the ordinary exact-head code and explicit security reviews, required checks,
CodeQL baseline, resolved-thread and protected-merge gates. It does not permit
tag movement, artifact substitution, a different release PR, or any future
release without its normal pre-merge maintainer attestation.

After protected merge, the recovery workflow rebuilds the immutable v0.20.1
source, produces the complete native archive set, checksum manifest, SPDX SBOM
and publication receipt, attests them, creates the immutable tag and release,
and verifies the public result.
