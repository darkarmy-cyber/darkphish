# Reviewed merge and release interlock

Starting with 0.8, automation performs a single protected squash merge only after
all ten required checks succeed, CodeQL has no open alerts, and both review
passes certify the current PR head. It never grants a native queued auto-merge
permission which a later push could inherit. This changes release governance,
not application behavior, database schema, audit formats or runtime dependencies.

## Evidence and trust boundary

`scripts/review-gate.mjs` recognizes the observed Codex connector summary and
explicit clean code/security result comments. It requires the fixed connector
bot identity and GitHub App identity, the correct repository/PR and complete
security-review head SHA, completed status for both rows, a manually requested
security review, and clean results for the same head. Short result SHAs must
resolve through GitHub to the complete expected SHA; ref-name shadowing and
ambiguous resolution block the operation. A reaction or a user-posted copy of a
bot message is not review evidence.

Comment creation identity alone is insufficient because maintainers can edit
comments. A final GraphQL read checks the current author/editor identities and
compares the exact content and timestamps of all three evidence comments. All
review-thread pages must be resolved, including outdated threads. Outstanding
GitHub changes-requested decisions and pending reviews block merging. Permission,
API, pagination, malformed-data or provenance failures stop the operation.
Unchanged pending evidence is reported as waiting, not as a successful review.

The [official code-review instructions](https://learn.chatgpt.com/docs/third-party/github)
and [explicit security-review instructions](https://learn.chatgpt.com/docs/security/security-review)
describe separate review requests and reporting thresholds. They do **not**
promise a stable machine-readable summary schema. This adapter therefore fails
closed on format drift; update it only through tested, reviewed engineering.
GitHub-visible findings reflect configured reporting thresholds, not proof that
every possible weakness or every item in the private cloud report is absent.
This batch does not change Codex settings or thresholds.

## Protected merge and durable pause

`mergeReviewedPullRequest` checks the live internal PR, protected private default
main, repository auto-merge policy, opt-in label, all checks and CodeQL before
review validation. It rechecks PR/head/base/readiness and checks afterward. Any
observed legacy queued auto-merge is revoked before evaluating pending evidence,
even if the head changed. Draft PRs and PRs without `codex-automerge` do not merge.

The final synchronous [GitHub merge request](https://docs.github.com/en/rest/pulls/pulls#merge-a-pull-request)
contains `sha` equal to the full reviewed head and `merge_method: squash`. GitHub
still enforces branch protections and rejects a changed head. No administrator
bypass, merge-queue insertion, manufactured approval or new required-check name
is used. Native protection still owns concurrent check/thread/branch changes.
Comment APIs and merge are not one atomic transaction; the adapter rechecks
evidence but cannot make external review-provider updates atomic with GitHub.
The full-head compare-and-swap prevents merging a later unreviewed code push.

Remove `codex-automerge` (or mark a PR draft) to pause automated merging. Release
preparation applies the label only when it creates a new release PR, never during
recovery of an existing one. Re-add it explicitly after reviews and readiness are
appropriate. Recovery may still validate/update generated changelog content and
dispatch checks while merge is paused; the pause is specifically a merge pause.

## Workflow isolation and review requests

Engineering reconciliation runs on a bounded schedule, manual dispatch and PR
metadata events. The `pull_request_target` workflow and its scripts come from
protected main; it never checks out PR code, installs its dependencies, restores
its caches or executes its artifacts. It rejects external forks and non-team
authors. Only read permissions needed for evidence supplement the existing
contents/PR merge permissions; checkout credentials are not persisted. See
[GitHub's privileged-workflow guidance](https://docs.github.com/en/actions/reference/security/securely-using-pull_request_target).

Bot-generated release PRs remain owned by release-prepare's exact-generated-diff
recovery path and use the same merge interlock. A maintainer or the approved
desktop continuation requests `@codex review` and `@codex security review` on
each final head. Workflows do not impersonate a reviewer or assume that a
GitHub Actions bot has a Codex workspace identity. Missing reviews stay pending;
scheduled recovery can merge once real evidence and all protections are ready.
Workflow-execution approval for a held bot PR remains separate from PR review.
Green dispatched tests never substitute for an unapproved required PR run.

Publication also verifies that both review completions preceded the release
PR's merge. It checks before building and again before exposing the draft,
alongside existing exact-main CI, CodeQL and immutable-asset checks. Human review
approval count and repository/tag protection settings are unchanged; this is an
automation/publication interlock, not a new GitHub organization policy.

The first rollout PR can be merged by the maintainer's existing protected,
full-head operation after all checks and both reviews, because the new script
does not exist on protected main until that merge. Do not run its unmerged code
with privileged workflow credentials to bootstrap the interlock.
