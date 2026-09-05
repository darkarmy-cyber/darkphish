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
target the current unreleased version or exactly the next minor. CI validates
the fragment and requires one for runtime changes. After merge to a protected
main branch and successful CI, release preparation aggregates fragments into
`CHANGELOG.md`, removes consumed files, and opens one `release/vX.Y.0` pull
request. Recovery fragments extend the existing unreleased section without
duplicating entries. Darkphish 0.3 recovery does not advance VERSION.
Required checks and repository approval policy remain authoritative.

After the release pull request merges, the native release workflow reruns tests,
builds the supported OS/architecture archives with embedded build metadata,
generates SHA-256 checksums and an SPDX SBOM, optionally attests artifacts when
the private-repository plan supports it, creates the
`vX.Y.Z` tag, and publishes the GitHub Release. No release is published when a
required stage fails, and container publication is not part of this process.

## Automated lifecycle and recovery

Feature/fix branch → PR → CI → trusted `codex-automerge` eligibility → protected
merge → green main → release preparation → release PR → the same required checks
→ protected release merge → native binaries → draft verification → `vX.Y.0`.

The single-maintainer policy requires PRs and checks, resolved conversations and
up-to-date branches, with zero outside approvals. No workflow fabricates a review.
See [repository administration](REPOSITORY_ADMIN.md) for exact external settings,
required check names, token permissions and unavailable private-plan features.

Preparation runs after completed CI/CodeQL or manual dispatch. A 15-minute
reconciliation schedule also dispatches missing main checks when a GitHub-token
merge suppresses push events. Failed checks are not automatically repeated
without a code change or explicit rerun. Release PRs receive explicit CI/CodeQL
dispatches using ephemeral GitHub tokens; no personal PAT is required.
Publication waits for green checks on the resulting main merge.

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
manifest, uploads to a draft, verifies uploaded digests, then publishes. An
unexpected tag/release stops publication. A known incomplete draft may resume:
identical assets are reused; different assets are never replaced. Rerun failed
publication jobs to reuse original build artifacts. If a full rerun produces
different archive bytes, investigate the draft instead of overwriting them.

The embedded UTC build timestamp comes from the protected release commit's
committer timestamp and stays stable across retries. Binaries include version
and SHA and use `-trimpath`. Tags are immutable; GitHub removes merged release
branches. Recovery never automatically bumps to 0.4.
