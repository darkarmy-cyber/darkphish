# v0.27.2 recovery after unapproved generated PR #254

Status: proposed recovery input. This document is not a release, tag, review, approval, attestation, or authorization to bypass protection. It supersedes `RECOVERY_V0_27_2_POST_252.md`.

## Verified incident state (2026-11-20 UTC)

- Bot-authored generated release PR #254 had head `a86322272b18f9735e3a14a336f33d75a56ed818` and base `961fe5400738670d4acbafd3cd4b9f2c71147510`. CI, CodeQL, and Codex code and security reviews completed on that head.
- The only submitted GitHub review was `COMMENTED` with body `OK` by `oliverkko` at 16:14:56Z. `oliverkko` merged PR #254 at 16:15:12Z as `d0e1f476811a2f562f2fd971d59d353a33a0bb43`. No exact-head pre-merge `APPROVED` review exists for #254; a later approval cannot repair that chronology.
- The v0.27.2 tag and release endpoints return 404. Native release metadata fails. Recovery run 44782257918 failed closed because merged release PRs #254, #252, #250, #248, and #244 exist with no tag or staging draft.
- The repository has one maintainer and deliberately requires zero approvals for ordinary owner-authored engineering PRs. Generated release PRs are bot-authored so the same maintainer can personally submit their required `APPROVED` review before the protected release merge.

## Safe recovery sequence

1. Review and merge this recovery-input PR through the ordinary protected route after exact-head CI, CodeQL, and independent code and security reviews complete. It is owner-authored and does not require a self-review.
2. Wait for protected-main CI and CodeQL. Let `Prepare release changelog` create a fresh bot-authored `release/v0.27.2` PR. Verify its generated-file-only diff and record its current PR number, exact head, and base.
3. For that generated PR, complete exact-head CI, CodeQL, independent code and security reviews, and resolve findings and threads. Keep GitHub native auto-merge disabled on this PR. Do not click `Merge` or `Enable auto-merge`.
4. `oliverkko` personally opens **Files changed → Review changes**, enters the exact body produced by `releaseReviewBody` for the current PR, selects **Approve**, and submits the review. A plain comment such as `OK`, a `Comment` review, or approval copied from an earlier PR does not satisfy the release gate.
5. Leave the protected `Prepare release changelog` workflow to verify the approval and synchronously merge the generated PR. The repository-level `Allow auto-merge` capability must remain enabled for this guarded path, while no PR-level native auto-merge is queued.
6. Only the native release workflow may build, attest, tag, and publish. Verify the tag, source commit, platform archives, SPDX SBOM, checksums, publication receipt, and workflow attestations. Only then prepare PR #246 for v0.27.3.

If any source, review, check, branch, tag, release, or queue state differs, investigate before proceeding. Never create a manual tag or attest to a review that did not occur.
