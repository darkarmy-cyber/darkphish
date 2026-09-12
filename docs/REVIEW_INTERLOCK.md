# Reviewed merge and release interlock

Darkphish automation performs a single protected squash merge only after all required checks succeed, CodeQL gates pass, and both trusted review passes certify the current PR head. It never treats a queued merge permission as durable approval for later code.

## Evidence and trust boundary

`scripts/review-gate.mjs` recognizes the observed Codex connector summary and explicit clean code/security result comments. It verifies the connector bot and GitHub App identities, the repository and pull request, the complete security-review head SHA, completed state for both review types, an explicit manual security-review request, and clean results for the same current head.

Short display SHAs must resolve to the complete expected commit. Ref shadowing, ambiguous resolution, edited or forged evidence, API failures, malformed pagination, unresolved threads, pending reviews, changes-requested decisions, stale results, or later superseding connector results fail closed. Reactions and maintainer-authored copies of bot messages are never review evidence.

Historical findings may be retained in a connector summary only when the adapter can map them unambiguously to authentic earlier review evidence and every corresponding thread is resolved. Fresh clean code and security results must postdate the historical finding. Unknown or current findings block merge.

## Protected public repository merge

`mergeReviewedPullRequest` requires the live repository to be the standalone public `darkarmy-cyber/darkphish` repository with protected default branch `main`. It verifies repository identity, public visibility, non-fork status, exact PR head/base snapshots, required checks, CodeQL state, review evidence, resolved conversations, mergeability, draft state and the `codex-automerge` opt-in label.

If GitHub reports a pre-existing native auto-merge, automation revokes it before evaluating current evidence. The final merge request is synchronous, uses `merge_method: squash`, and includes the complete reviewed head SHA. GitHub branch protection remains authoritative. No administrator bypass, manufactured approval or unreviewed queued merge is allowed.

Remove `codex-automerge` or mark a PR draft to pause automated merging. A later push invalidates exact-head review evidence and requires fresh reviews.

## Workflow isolation

Privileged merge/release workflows execute protected-main automation code only. They do not execute untrusted pull-request code with elevated `pull_request_target` privileges. Generated release PRs are subject to the same exact-head review and resolved-thread requirements as engineering PRs.

A maintainer or approved automation explicitly requests both `@codex review` and `@codex security review` on each final head. Workflow execution approval, where GitHub requires it, is separate from PR review evidence and cannot substitute for a review.

## Publication gate

Release publication verifies that the merged generated release PR was reviewed on its exact head before merge, that protected `main` still corresponds to the expected release source, and that required CI, CodeQL, native artifacts, checksums and SBOM validation succeed. Unexpected or mutable tag/release state fails closed.

The interlock is deliberately conservative: inability to prove the required state is treated as a blocker, not as permission to merge or publish.
