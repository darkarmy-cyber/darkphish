# v0.27.2: fail-closed release recovery authorization and acceptance criteria

Status: **proposal only — not an executable recovery or replacement for a GitHub review**.

## Immutable incident evidence

- Repository: `darkarmy-cyber/darkphish`; protected branch: `main`.
- Already merged generated release PR: [#244](https://github.com/darkarmy-cyber/darkphish/pull/244).
- Generated release head: `cc600c1cfb741a30cf3b4d85917c1d57545ae49c`.
- Original release merge commit: `3f103ded1bcae7e8eccaaf46c81b910c591586cd`.
- The PR merged on 2026-11-19T13:55:57Z and has **zero recorded GitHub PR reviews**.
- Protected-main CI and CodeQL passed at the merge commit, but **that does not substitute for the required, pre-merge, exact-head generated-release maintainer attestation**.
- Native release run `44539086459` was stopped at metadata verification; recovery run `44539086267` also failed closed.
- No trusted `v0.27.2` GitHub Release was published as of initial investigation.

## Scope and explicit safeguards

This document records the repository owner's express authorization to **prepare** a narrowly scoped recovery, including a new release PR, subject to the existing trusted review and publication gates. It is **not** the attestation itself and cannot grant GitHub reviewer identity or historical approval.

1. Keep the original PR, its merge commit, and evidence immutable. Never manufacture, backdate, edit, or substitute a review for PR #244.
2. Maintain the freeze of ordinary engineering and Dependabot merges while `VERSION=0.27.2` remains unpublished.
3. Implement any recovery exception as a separate, narrowly scoped, reviewed code change with explicit immutable version, source SHA, original PR number, and pre-existing failure provenance. No generic release-check bypass, no broad permission increase, and no write to protected main outside normal PR protection.
4. Generate a **new** same-version release PR from the audited current protected source. The new PR must contain only legitimate regenerated release metadata, with deterministic file/commit identity; an empty/no-op PR or a hand-edited release tag is not a substitute.
5. The new generated release PR must receive all required CI and CodeQL checks and a real exact-head code and security review, plus the required maintainer attestation **before** its protected merge. Verify the immutable GitHub review identity, commit and chronology.
6. Publish `v0.27.2` only through native verified artifacts, checksums, SPDX SBOM, post-gate receipt, and original publication/attestation checks. Release and tag must remain immutable with no overwrite of arbitrary artifacts.
7. Only after successful publication and verification, resume PR #245 (dependency-review gate) and PR #246 (`fast-uri` 3.1.9, four Dependabot alerts). Require fresh checks and confirm alerts close after merge.
8. Preserve regression coverage for failure on absent or post-merge review, wrong reviewer, stale source, changed release manifest, missing code/security checks, and invalid release receipt.

## Engineering blocker

The normal `scripts/release-prepare.mjs` requires an unconsumed changelog fragment and `assertReleaseAdvancePublished`, so it cannot simply re-create a second `v0.27.2` release PR from the already-advanced protected `VERSION`. Existing `scripts/release-publish.mjs` and `.github/scripts/release-recover.mjs` bind publication to a previously verified release PR. An isolated recovery implementation must account for both invariants; merely re-running the existing workflow cannot succeed.

**Do not merge this proposal as if it were the recovery implementation.**
