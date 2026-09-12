# Version and release policy

`VERSION` is the authoritative Darkphish SemVer value. Release tags add `v`
(for example, `0.3.0` becomes `v0.3.0`), while the product UI and CLI shorten
normal releases to `Darkphish 0.3`. Unreleased binaries identify themselves as
`0.3-dev`; release builds also contain the commit SHA and UTC build timestamp.

Darkphish has an independent version history. Normal engineering batches
advance the minor component (`0.2.0` to `0.3.0`); patch releases are reserved
for exceptional fixes to an already released minor. The project never derives
its next number from historical Gophish versions.

Every runtime pull request adds a structured file under `changes/`. It may
target the current unreleased version, exactly the next minor, or the next patch
of the current released minor when the change is an exceptional production
hotfix. Patch hotfixes must remain narrowly scoped to regression, security,
data-integrity, or production-blocking fixes and must not introduce unrelated
features. CI validates the fragment and requires one for runtime changes. After
merge to a protected main branch and successful CI, release preparation
aggregates fragments into `CHANGELOG.md`, removes consumed files, and opens the
corresponding release pull request. Recovery fragments extend the existing
unreleased section without duplicating entries. Required checks and repository
approval policy remain authoritative for patch releases exactly as for normal
minor releases.

After the release pull request merges, the native release workflow reruns tests,
builds the supported OS/architecture archives with embedded build metadata,
generates SHA-256 checksums and an SPDX SBOM, optionally attests artifacts when
the repository plan supports it, stages the exact artifact set in a non-public
draft, revalidates protected-main, review/source and CodeQL gates, writes a
post-gate publication receipt, and only then publishes the `vX.Y.Z` GitHub
Release. No release becomes public when a required stage fails, and container
publication is not part of this process.

## Automated lifecycle and recovery

Feature/fix branch → PR → CI → trusted `codex-automerge` eligibility → protected
merge → green main → release preparation → release PR → the same required checks
→ protected release merge → native binaries → draft verification → post-gate
publication receipt → `vX.Y.Z`.

The single-maintainer policy requires PRs and checks, resolved conversations and
up-to-date branches, with zero outside approvals. No workflow fabricates a review.
See [repository administration](REPOSITORY_ADMIN.md) for exact external settings,
required check names, token permissions and unavailable plan features.

Preparation is triggered only from trusted `main` CI/CodeQL workflow completion
or the non-cancelling reconciliation schedule. Branch-dispatchable write paths
are intentionally absent. The reconciliation schedule can dispatch missing main
checks when a GitHub-token merge suppresses push events. Failed checks are not
automatically repeated without a code change or an explicit GitHub rerun of the
existing trusted workflow. Release PRs receive explicit CI/CodeQL dispatches
using ephemeral GitHub tokens; no personal PAT is required. Publication waits
for green checks on the resulting main merge.

Before remote mutation, preparation verifies protected main, all checks,
tag/release absence, fragments, branch ancestry and existing PR state. The
PR-creation policy is probed where possible; otherwise failure names the exact
organization/repository setting. The generated branch remains reusable.

Existing `release/v0.3.0` commit `7c613c7` is accepted only if its author, message,
paths and exact output match trusted regeneration from its base. A refresh
retains the old tip as a parent and uses a normal fast-forward push. No force
push or unknown developer-work replacement is permitted. Matching PRs are reused.

Publication accepts only current protected main associated with a merged
generated release PR. It verifies all five native archives, the SBOM and checksum
manifest, uploads them to a trusted draft and verifies uploaded digests. Starting
with v0.7.1, the workflow then rechecks protected main, exact review/source state
and CodeQL before uploading `darkphish-vX.Y.Z.release.json`. That receipt binds
the release tag and source SHA to the SHA-256 digest of `SHA256SUMS`; it is part
of the trusted published asset set but is deliberately not listed inside
`SHA256SUMS` because it is created only after the checksum manifest and final
gates have already been verified. A subsequent patch accepts the current
release only when the receipt is present, was uploaded by the canonical GitHub
Actions actor, and its contents match the current release and checksum manifest.
Legacy v0.7.0 remains compatible with its original seven-asset release shape.

An unexpected tag/release stops publication. A known incomplete Actions-created
draft may resume: identical assets are reused; different assets are never
replaced. If a maintainer publishes the draft before the final gates and receipt,
the workflow does not adopt that publication as trusted evidence for a later
patch. Rerun failed publication jobs to reuse original build artifacts. If a full
rerun produces different archive bytes, investigate the draft instead of
overwriting them.

Once a generated release PR advances `VERSION`, ordinary protected engineering
merges and Dependabot merges are frozen until that exact current version has a
trusted published release. This durable boundary covers the interval before the
publication workflow starts as well as the publication run itself; the release
PR merge uses the dedicated release path so it does not deadlock on its own
future publication.

The embedded UTC build timestamp comes from the protected release commit's
committer timestamp and stays stable across retries. Binaries include version
and SHA and use `-trimpath`. Tags are immutable; GitHub removes merged release
branches. Recovery never automatically bumps to the next minor.
