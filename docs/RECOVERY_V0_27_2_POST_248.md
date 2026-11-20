# v0.27.2 release recovery after prematurely merged PR #248

Status: **draft recovery proposal; not an authorization to merge, override protection, or publish a release.**

## Evidence observed on 2026-11-19 (UTC)

- Generated release PR #244 was merged at 13:55:57Z with head `cc600c1cfb741a30cf3b4d85917c1d57545ae49c`, without the required qualifying pre-merge generated-release maintainer attestation.
- The narrowly scoped initial repair PR #247 merged at 18:06:39Z (merge commit `1c9d702c9dc2bd18a918633978a8e28099ec7c93`), enabling trusted preparation of a new generated release PR.
- Generated release PR #248 used head `fd37e0a90a1bed02d79638b4fe96387f9eb18f9a` and base `1c9d702c9dc2bd18a918633978a8e28099ec7c93`. Its diff contained only `CHANGELOG.md` aggregation and deletion of `changes/recover-v0-27-2-attestation.md`.
- CI and CodeQL for that head succeeded, and an independent Codex code review recorded no findings. However, the Codex security-review receipt remained absent and the GitHub pull-request reviews API returned no submitted `APPROVED` maintainer review.
- Despite these missing safeguards, GitHub records PR #248 as merged by `oliverkko` at **18:18:34Z**, merge commit `478fc4b53000157069b0ef76c751982b23da6a19`. This cannot be cured by retrospective approval.
- At the initial post-merge inspection, `main` was `478fc4b53000157069b0ef76c751982b23da6a19`, `VERSION` remained `0.27.2`, no `v0.27.2` tag or public GitHub Release existed, and push CI/CodeQL on that merge were still running. Recheck those facts before any subsequent operation.

Source references: GitHub PRs #244, #247, #248, their review timelines, the exact-head workflow runs, and the GitHub release/tag endpoints.

## Scope and safety boundary

This proposed repair introduces **only** this incident document and an existing-version `0.27.2` Security changelog fragment. It makes no runtime-code, CI-workflow, token-permission, tag, or ruleset changes. Do not force push, delete/rewrite protected refs, alter immutable tags, fabricate review evidence, or submit an owner `APPROVED` review using automation.

The new repair PR must remain **draft** until independent code and security reviews on its exact head are both genuinely complete and clean, required GitHub CI/CodeQL checks pass, outstanding review threads are resolved, and an authorized human maintainer explicitly decides to perform the one-off protected recovery merge under the existing ruleset. The original special authorization for PR #247 does not automatically approve a different repair PR.

Before that manual repair merge, verify `main`, `VERSION`, absence of a published/tagged `v0.27.2`, and the precise two-file scope; verify no other release-recovery PR or release branch has already been generated. If any evidence differs, stop and revise the plan, rather than overwriting an existing ref.

After a legitimate, reviewed repair merge, wait for green exact-main CI and CodeQL and let the trusted `Prepare release changelog` workflow create a **new** generated release PR for `0.27.2`. On that generated PR require independent exact-head code and security reviews, all required checks, and a real GitHub `APPROVED` review by the pinned maintainer with the exact-head/base generated-release attestation. Record all evidence **before merging** that PR. Never reuse the approvals or source identity of PR #244 or PR #248.

Only the trusted native release workflow can stage, build, attest, and publish `v0.27.2` with the immutable tag and verified assets. After verifying release provenance, proceed independently with PR #245 (dependency review gate) and PR #246 (fast-uri 3.1.9) under fresh CI/CodeQL and genuine reviews; release `v0.27.3` and confirm the four advisories are actually closed.

## Important repository protection gap

At the time of this incident, the repository branch ruleset required status checks and resolved discussions but had `required_approving_review_count: 0`; therefore a manually initiated GitHub merge can occur without the stronger **application-level** pre-release review and attestation policy. The repository owner should address this mismatch through a separately reviewed GitHub protection design before authorizing another generated release merge. Do not weaken other required checks or disable the protected merge path.
