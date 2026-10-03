# Release repair: stranded v0.25.1

Darkphish 0.25.1 was merged by generated release PR #196 but cannot be published because the required exact-head maintainer attestation was not recorded before merge. The native publisher correctly refuses retrospective approval, and no v0.25.1 tag or release exists.

PR #203 is the narrowly scoped repair boundary. It may advance the repository from the stranded 0.25.1 state to patch release 0.25.2 only when:

- the base commit is exactly `98d96c7cc7eebf6ab09bbeffdf109778dd4f3632`;
- release PR #196 remains the pinned merged source and v0.25.1 remains tagless/unpublished;
- the PR changes only the enumerated repair-policy, audit-policy, tests, documentation and changelog files;
- normal CI, CodeQL, review and protected-merge gates succeed.

The security change documents a temporary pnpm audit exception for GHSA-vfj7-8cjw-p6xm because the upstream `braces` package currently has no patched release. The dependency is transitive development tooling, not a Darkphish runtime path. The exception must be removed when upstream publishes a fix.

After this repair merges, normal release preparation must create a fresh generated v0.25.2 release PR. That release PR must receive all exact-head reviews and maintainer attestation before merge; no retrospective review exception is authorized.

## Scheduled preparation continuation

The first repair merge exposed a narrow automation gap: `scripts/changelog.mjs` could authorize the repair on pull-request and push events, but scheduled release preparation has no `event.before` SHA. The continuation repair permits scheduled/workflow-run preparation only when the protected main history is the exact reviewed continuation of repair commit `4e24f93277cfc5c14fbfec63a797436018c495f1`, the repository is still on VERSION 0.25.1 with only 0.25.2 fragments, and the continuation merge matches its pinned branch, title and file set.

This continuation does not authorize publication by itself. `scripts/release-prepare.mjs` still verifies the remote stranded v0.25.1 identity through `assertReleaseAdvancePublished`, then creates a fresh generated v0.25.2 release PR. That generated PR must pass normal CI, CodeQL, connector reviews and the maintainer attestation before merge.

