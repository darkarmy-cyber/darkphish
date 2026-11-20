# v0.27.2 recovery after unapproved generated PR #252

Status: proposed recovery input. This document is not a release, tag, review, approval, attestation, or authorization to bypass protection. It supersedes the procedure in `RECOVERY_V0_27_2_POST_250.md`.

## Verified incident state (2026-11-20 UTC)

- Generated release PR #252 was authored by `github-actions[bot]`, head `219059c8e220ae4f76e8bb69c4f9395341189843`, base `bb62b7105b8b53fd764272b01f6d6997a07b78b9`. It merged at 14:02:05Z as `90d4eb1beaccebaeac79e9c11cf5888c63971041`.
- GitHub's PR reviews list contains no submitted review for #252. GitHub records native auto-merge enabled by `oliverkko`; successful checks triggered the merge before the required maintainer attestation. Later approval cannot establish pre-merge provenance.
- Native release run 44750267420 failed in metadata: `Release PR #252 is missing the required exact-head generated-release attestation from oliverkko (309485696) before merge; publication remains blocked`.
- Recovery run 44750267789 failed closed: four merged release PRs (#252, #250, #248, #244) exist with no tag or staging draft. The `v0.27.2` tag and release endpoints return 404. The `release/v0.27.2` branch ref is absent.
- The active `main-protection` ruleset requires zero GitHub approvals. This matches the repository's single-maintainer policy in `REPOSITORY_ADMIN.md`; changing it globally to one approval would prevent `oliverkko` from merging owner-authored engineering PRs without a second maintainer.
- The working solo-maintainer pattern is visible on generated release PR #240: the bot authored the PR, `oliverkko` submitted an exact-head GitHub `APPROVED` review before merge, and the protected release process accepted it.

## Safe recovery sequence

1. Review this recovery-input PR on its exact head. Require green CI and CodeQL, genuinely completed independent code and security reviews, resolved findings and threads, and a recheck of protected `main`, `VERSION=0.27.2`, absent tag/release, and absent `release/v0.27.2` branch. This owner-authored PR cannot receive its author's own GitHub approval. The authorized maintainer may personally perform an ordinary protected squash merge only after those conditions hold.
2. Let protected-main CI and CodeQL complete. `Prepare release changelog` may then create a fresh generated `release/v0.27.2` PR authored by `github-actions[bot]`. Its diff must contain only generated changelog aggregation and consumed-fragment removal.
3. **Never queue GitHub native auto-merge for this generated PR.** Leave the repository's `Allow auto-merge` capability enabled because the guarded merge code checks it, but do not click `Enable auto-merge` or `Merge` on the PR. If the PR shows a queued auto-merge, use `Disable auto-merge` immediately and recheck that it remains open.
4. Complete exact-head CI/CodeQL, independent code and explicitly requested security reviews, and resolve every finding and thread. Confirm the PR's exact head and base still match the evidence.
5. The pinned maintainer `oliverkko` personally reviews the bot-authored PR and submits a GitHub **Approve** review **before merge**, using the exact body produced for that PR by `releaseReviewBody` in `scripts/release-maintainer-review.mjs`. The body must bind the current PR number, head, base, repository, and completed review claims. An ordinary approval or a body copied from #252 is insufficient.
6. Leave the protected `Prepare release changelog` workflow to call its review-verified synchronous merge path. Do not queue native GitHub auto-merge or use administrator bypass. Keep later main changes behind the publication.
7. Only the native release workflow may build, attest, create the immutable tag, and publish. Verify its source, tag, five platform archives, SPDX SBOM, SHA256SUMS, publication receipt, and workflow attestations before preparing PR #246 for v0.27.3.

If any source, review, check, branch, tag, release, or queue state differs, stop and investigate rather than reusing old evidence or creating a manual tag.
