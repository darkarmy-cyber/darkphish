# Version and release policy

`VERSION` is the authoritative Darkphish SemVer value. Release tags add `v`
(for example, `0.7.1` becomes `v0.7.1`), while the product UI and CLI may shorten
normal minor releases for display. Unreleased binaries identify themselves with
`-dev`; release builds also contain the commit SHA and UTC build timestamp.

Darkphish has an independent version history. Normal engineering batches advance
the minor component; patch releases are reserved for exceptional fixes to an
already released minor.

Every runtime pull request adds a structured file under `changes/`. It may target
the current unreleased version or an explicitly supported next release target.
CI validates fragments and requires one for runtime changes. After merge to a
protected main branch and successful CI, release preparation aggregates the
selected fragments into `CHANGELOG.md`, removes only the consumed fragments, and
opens one `release/vX.Y.Z` pull request. Strictly future fragments may remain for
a later release. Required checks and repository approval policy remain authoritative.

After the release pull request merges, the native release workflow reruns tests,
builds the supported OS/architecture archives with embedded build metadata,
generates SHA-256 checksums and an SPDX SBOM, creates the immutable `vX.Y.Z` tag,
and publishes the GitHub Release. No release is published when a required stage
fails.

## Automated lifecycle and recovery

Feature/fix branch → PR → CI → trusted `codex-automerge` eligibility → protected
merge → green main → release preparation → release PR → the same required checks
→ protected release merge → native binaries → draft verification → published tag.

The single-maintainer policy requires PRs and checks, resolved conversations and
up-to-date branches, with zero outside approvals. No workflow fabricates a review.
See [repository administration](REPOSITORY_ADMIN.md) for exact external settings,
required check names and token permissions.

Preparation runs after completed CI/CodeQL or manual dispatch. A reconciliation
schedule can dispatch missing main checks when a GitHub-token merge suppresses
push events. Failed checks are not automatically repeated without a code change
or explicit rerun. Release PRs receive explicit CI/CodeQL dispatches using
ephemeral GitHub tokens; no personal PAT is required.

Before remote mutation, preparation verifies protected main, all checks,
tag/release absence, fragments, branch ancestry and existing PR state. The
PR-creation policy is probed where possible; otherwise failure names the exact
organization/repository setting. Generated branches remain reusable only when
their ancestry and generated contents match the trusted release state.

Publication accepts only current protected main associated with a merged
generated release PR. It verifies all supported native archives, the SBOM and
checksum manifest, uploads to a draft, verifies uploaded digests, then publishes.
An unexpected tag or release stops publication. A known incomplete draft may
resume: identical assets are reused; different assets are never replaced. If a
full rerun produces different archive bytes, investigate the draft instead of
overwriting them.

The embedded UTC build timestamp comes from the protected release commit's
committer timestamp and stays stable across retries. Binaries include version
and SHA and use `-trimpath`. Tags are immutable. Recovery never silently advances
the release version.
