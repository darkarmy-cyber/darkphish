# v0.27.2 recovery after generated release PR #250

Status: proposed recovery input only. This document does not authorize a merge, review, tag, release, or attestation.

## Observed evidence (2026-11-20 UTC)

- Generated release PR #250 merged at 10:44:57Z as `bcb16bd6752aee1929c31763671f5c5a4afe7d73`, from head `b46d68edf1ec6a84b278452663f3c818fb8dea9c` and base `e255de39b60e0bd671364a2f5db688c9e2870b91`. Its GitHub review list and discussion timeline contain no submitted reviews. The required pre-merge exact-head maintainer attestation cannot be supplied retrospectively.
- PR #245 then advanced protected `main` to `c8f506beadb14478181fd9f4ccf93de6c112b606`. `VERSION` is `0.27.2`; the `v0.27.2` tag and release endpoints return 404.
- Native release run 44723572977 checked out `c8f506b` and reported `No unpublished, fully checked release merge is ready.` The publisher selects only the current protected-main merge from a generated release PR and verifies its maintainer review before building.
- Recovery run 44723573084 stopped before building because it found merged release PRs #250, #248, and #244 but no tag or staging draft. None of those merges can be selected by guessing or by inventing an approval or build provenance.
- The active `main-protection` ruleset has zero required approving reviews, although the publication code requires an exact-head pre-merge maintainer `APPROVED` attestation. This settings mismatch allowed the historical merge to happen without satisfying the publication policy.

## Proposed protected path

1. Review this two-file PR on its exact head, including independent code and security review and green CI/CodeQL. The authorized maintainer decides whether to merge it through the existing protected PR route. Recheck `main`, `VERSION`, the absent tag/release, and other open release PRs immediately before merging. Do not merge if that state has changed.
2. The repository owner fixes the protection mismatch before any newly generated release PR is merged: require a real approval in the `main-protection` ruleset (or an equivalent required check that validates the pre-merge attestation). Preserve the other branch and immutable-tag rules. This settings action needs an authorized human; this PR cannot change it.
3. After the recovery-input PR merges and exact-main CI/CodeQL pass, `Prepare release changelog` may create a fresh `release/v0.27.2` PR from the new protected main. The new PR must contain only generated changelog changes and consumed-fragment removal. Do not reuse #244, #248, or #250 as the release source.
4. Independently inspect the new PR's exact head and base, complete genuine code and security reviews, resolve threads, and pass required checks. The pinned maintainer personally submits the prescribed GitHub `APPROVED` review for that exact head and base **before** the protected merge. A review on this recovery-input PR does not count as a generated-release attestation.
5. Let `Native release` build, attest, and publish from the newly reviewed release merge. Keep later main changes behind this publication. Verify the immutable tag target, release source, complete five-platform archives, SPDX SBOM, SHA256SUMS, publication receipt, and workflow attestations before advancing to PR #246 for v0.27.3.

No manual tag creation, ruleset bypass, postdated review, synthetic security receipt, or reuse of previous release artifacts is authorized here.

