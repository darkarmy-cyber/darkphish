# v0.27.2 release recovery — authorized manual boundary

Status: **approved recovery plan; not an authorization to disable GitHub branch protection**.

## Verified original evidence

- Original generated release PR: #244, closed and merged at 2026-11-19T13:55:57Z.
- Original generated head: `cc600c1cfb741a30cf3b4d85917c1d57545ae49c`.
- Original merge / protected main base: `3f103ded1bcae7e8eccaaf46c81b910c591586cd`.
- Original PR #244 has no qualifying pre-merge exact-head maintainer attestation.
- Native release workflow `44539086459` and recovery workflow `44539086267` failed closed; do not rerun as if they can repair lost historical approval.
- Original `release/v0.27.2` branch is absent as of the 2026-11-19 check. Confirm again immediately before performing recovery.

## Explicit recovery boundary

An engineering PR cannot make new untrusted code authorize its **own merge**: `scripts/automerge.mjs` runs the already-protected `main` checkout, which does not know a newly added repair authorization. For that reason this recovery deliberately **does not add an ineffective one-shot verifier** and does not change protected automation, GitHub rulesets, or publication verification.

The repository owner has explicitly authorized preparing a one-time v0.27.2 recovery. The only exceptional step is a **manually initiated, GitHub-protected merge of this exact repair PR** after independently confirming the evidence below. This is **not** a generic automated bypass of the version-publication freeze; GitHub's ordinary required checks and protected-branch constraints remain mandatory.

The repair PR may contain **only**:
- `changes/recover-v0-27-2-attestation.md`, with `category: Security` and `version: 0.27.2`, making regenerated same-version release metadata possible;
- this incident and recovery document.

## Mandatory pre-merge procedure

1. Confirm `main` is still exactly `3f103ded1bcae7e8eccaaf46c81b910c591586cd`, `VERSION=0.27.2`, the original merged PR #244 fields above, and no `v0.27.2` tag, draft or published release.
2. Confirm the original `release/v0.27.2` branch is absent. If it has reappeared, **stop**, investigate its immutable GitHub ref and the closed PR; do not delete, rewind, overwrite, force-push, or reuse it automatically.
3. Verify this repair PR contains exactly the two allowed files and the unchanged source/version, with no unexpected additional commits or generated assets.
4. Require **all** exact-head mandatory GitHub Actions CI/CodeQL checks and independently completed, clean code/security reviews, including resolving every outstanding finding. Preserve immutable review evidence.
5. Only then may an authorized maintainer use GitHub's normal protected, exact-head merge operation to merge the recovery fragment. If branch protection or any check denies the merge, **stop** rather than bypassing rules.
6. After merge, require green CI and CodeQL on the new protected main commit. Let the trusted protected-main `release-prepare.yml` produce a fresh `release/v0.27.2` branch and **new** generated release PR. Never manually invent a release commit or reviewer evidence.
7. Require the fresh generated release PR to receive all exact-head CI/CodeQL checks, clean code and security reviews, and the explicit GitHub maintainer **APPROVED** exact-head generated-release attestation **before** its protected merge.
8. Only the native release pipeline may build, hash, SBOM-attest, draft, and publish `v0.27.2`. Verify release assets, receipt, protected source SHA and immutable tag.
9. Only after the trusted published `v0.27.2` is verified, rebase and complete separate PRs #245 (dependency gate) and #246 (`fast-uri` 3.1.9); require fresh checks, then confirm all four Dependabot alerts close and publish a separately verified `v0.27.3`.

**Fail closed:** any changed source, existing release branch/tag, missing review, invalid artifact, or unsatisfied protection makes the recovery incomplete. The original unreviewed PR #244 never becomes retrospectively authorized.
