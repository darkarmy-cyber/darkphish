# Repository administration

Darkphish is the public, standalone `darkarmy-cyber/darkphish` repository with default branch `main`. Preserve required legal attribution and dependency notices, but do not import unrelated upstream tags or releases. Published Darkphish tags are immutable.

## Protection and merge policy

Protect `main` with pull-request requirements, all status checks from `.github/required-checks.json`, an up-to-date branch, resolved conversations, administrator enforcement, disabled force pushes, and disabled branch deletion. Bind every required status-check context to the GitHub Actions application (app id 15368), not merely to its check name, so an identically named check from another integration cannot satisfy protection. Compare configured names and app/source bindings with the actual emitted check runs whenever protection is restored. The project uses a zero-outside-approval single-maintainer policy; workflows must never fabricate approvals or bypass protection.

For the zero-approval policy, `require_extra_approval_for_unattributed_changes` must remain **off** on the effective pull-request rule protecting `main`. Generated release PRs are authored by `github-actions[bot]` with author association `NONE`; enabling that extra-approval flag would make those release PRs require an approval the automation intentionally never supplies. Verify this setting read-only before preparing a bot-authored release; do not weaken any other protection to make a release pass.

Enable repository auto-merge support only as a platform capability. Darkphish automation itself uses the `codex-automerge` label as an explicit opt-in and performs a synchronous protected squash merge pinned to the complete reviewed head SHA. A queued native auto-merge, if encountered, is revoked before reevaluation.

Every automated engineering or generated-release merge requires:

- an internal PR targeting protected `main`;
- exact current head and base snapshots;
- every required GitHub Actions check successful;
- zero open CodeQL alerts for the protected source state where required;
- completed trusted code review for the exact head;
- completed explicitly requested security review for the exact head;
- no later superseding connector result;
- all review threads, including outdated threads, resolved;
- the `codex-automerge` label still present and the PR not draft.

See [REVIEW_INTERLOCK.md](REVIEW_INTERLOCK.md) for the evidence model.

## Public-repository security features

The repository is intentionally public. Private vulnerability reporting should be enabled where GitHub makes it available, and suspected vulnerabilities should be handled through GitHub Security Advisories rather than public issues. CodeQL, dependency alerts, secret scanning and push protection should remain enabled where supported by the active organization plan.

Never store GitHub credentials, signing keys, production secrets, customer data or private review artifacts in the repository. Workflow tokens should use the minimum permissions needed by each job.

## Release administration

At both the organization and repository levels, enable **Settings -> Actions -> General -> Workflow permissions -> Allow GitHub Actions to create and approve pull requests**. The organization policy must permit the repository setting. Before release preparation, verify that the effective repository permissions report `can_approve_pull_request_reviews: true`; a missing or false value is a blocking configuration error. This setting allows the ephemeral workflow token to create the generated release PR. Darkphish workflows still must not submit approval reviews.

Release preparation and publication execute code from protected `main`, validate exact source identity, and fail closed on missing checks, stale reviews, unresolved threads, unexpected tags/releases or inconsistent generated branches. Release PRs use the same exact-head review interlock as engineering PRs. Native artifacts, SHA-256 checksums, SPDX SBOMs and immutable release tags remain mandatory.

The release workflows may create pull requests, dispatch checks and publish verified release assets using ephemeral GitHub tokens. They must not obtain administrator bypass, ruleset-write permission or long-lived personal access tokens as a workaround for policy failures.

## CodeQL release baseline

`scripts/codeql-baseline.mjs` is a read-only release diagnostic. Both configured CodeQL languages must have successful latest analyses for the exact protected default-branch SHA and zero open alerts. API errors, stale analyses, malformed results or a changing default branch fail closed. The diagnostic does not dismiss alerts or weaken scanning policy.

## Supply-chain policy

Security-sensitive third-party Actions should remain SHA-pinned where the repository already requires that model. GitHub-owned maintained actions may follow the repository's reviewed dependency policy. Dependency updates affecting authentication, cryptography, databases, migrations, workflows or release tooling require engineering and security review.

Git source does not preserve repository rulesets, labels, organization policy, security-product settings or release assets. Administrators must back up and restore those settings separately without storing credentials or secrets in repository files.
