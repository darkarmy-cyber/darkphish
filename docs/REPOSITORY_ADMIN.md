# Standalone repository administration

Darkphish is the private, standalone `darkarmy-cyber/darkphish` repository with
default branch `main`. Preserve upstream Git ancestry and legal attribution, but
do not import Gophish tags/releases. The first official Darkphish GitHub release
is `v0.3.0`; 0.1 and 0.2 remain in Git and CHANGELOG. Recovery keeps VERSION 0.3.0.

## Protection and merge policy

Protect `main` under Settings → Branches (or an equivalent enforced ruleset):
require pull requests, all status checks, an up-to-date branch, resolved
conversations and administrator enforcement. Disable force pushes and deletion.
Require zero outside approvals for this single-maintainer project, dismiss stale
approvals when present, and leave mandatory code-owner approval disabled.
`.github/CODEOWNERS` names the verified maintainer `@oliverkko` for sensitive paths.

Use exactly the check names in `.github/required-checks.json`, bound to the GitHub
Actions application (15368). Compare the names with actual emitted check runs
before restoring protection. The Go analysis job includes formatting, build,
unit/security/SQLite tests, vet, staticcheck, govulncheck and clean-tree checks.
Race tests are separate. Database jobs include up/down/up and security-model
integration with failure injection. CodeQL runs Go and JavaScript/TypeScript.

Enable Settings → General → Pull Requests → Allow auto-merge and Automatically
delete head branches. Label eligible internal PRs `codex-automerge`. GitHub native
squash auto-merge remains subject to all protections. Workflows never submit
review approvals, impersonate reviewers, or use administrator merge bypasses.

The supported GitHub CLI merge-when-ready operation handles both pending checks
and already-green PRs. It is restricted to internal ready PRs targeting protected
main, requires repository auto-merge to remain enabled, and matches the expected
head SHA. It never uses an administrator override. GitHub's queue-only GraphQL
mutation rejects already-clean PRs, which otherwise breaks safe release retries.

Both preparation and publication have independent 15-minute scheduled preflights.
Hosted validation confirmed that checks explicitly dispatched by GITHUB_TOKEN can
finish without emitting the expected downstream workflow_run executions. The
publication schedule therefore reconciles a checked, merged release PR directly;
it still requires protected current main and every required check, and skips
builds when no unpublished release merge is eligible.

## Actions settings outside Git

Keep default workflow permissions read-only. Enable Settings → Actions → General
→ Workflow permissions → **Allow GitHub Actions to create and approve pull
requests**. GitHub combines PR creation and approval in this flag; Darkphish
uses only creation and never submits a review.

If the organization blocks the setting, an organization owner must first enable
the corresponding policy under organization Settings → Actions → General →
Workflow permissions, then enable it on this repository. During recovery the
repository token lacks `admin:org` / Actions policies permission, so changing the
organization policy is an external administrator action. Do not add a personal
PAT to repository secrets as a workaround.

CI has read-only contents access. CodeQL adds Actions read and security-events
write. Release preparation uses contents/PR/labels/Actions dispatch writes and
checks read. Publication uses contents write, Actions/checks/PR read and optional
OIDC/attestation permissions. Release scripts execute protected main code only;
metadata auto-merge never checks out PR code. No workflow executes untrusted PR
code under `pull_request_target` privileges.

## Security features and plan limitations

Restore Dependabot alerts/security updates, Code Security, secret scanning and
push protection where licensed. CodeQL advanced setup is defined in Git; do not
enable duplicate default setup. Dependabot checks weekly, but sensitive crypto,
authentication, database, migration and Action updates require engineering review.
Automatic dependency merging remains paused until main is fully green.

GitHub Team is the current organization plan. Code Security and Secret Protection
were enabled using existing allowances. Do not purchase seats or change the plan
while restoring the repository. Private vulnerability reporting is unavailable
for this private repository (API 404); collaborators use private advisories and
the Security tab. If visibility changes later, enable private reporting before
opening the issue tracker to the public.

Private-repository artifact attestations require Enterprise Cloud. Keep repository
variable `RELEASE_ATTESTATIONS_SUPPORTED` unset/false on Team. Native archives,
verified SHA-256 checksums, SPDX SBOM and protected release history remain
mandatory. This documented platform limitation does not disable any runnable
security scan. After an authorized plan change, set the variable to `true` to
activate `actions/attest@v4`. See [GitHub's availability rules](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations).

## Supply-chain and recovery policy

Security-sensitive third-party Actions use full SHA pins with version comments:
Trivy v0.36.0 (`ed142fd0673e97e23eac54620cfb913e5ce36c25`), pnpm setup v6 and
Anchore SBOM v0.20.6. Trivy transitively pins setup-trivy v0.2.6 and explicitly
runs scanner v0.70.0. GitHub-owned Actions retain maintained major tags under
Dependabot. Never follow floating branches. Review transitive Actions and
permissions when updating. Trivy scans filesystem secrets only; containers are
not release artifacts or product gates.

Git source/history does not preserve protection, security products, organization
policy, repository variables, labels, review rules or release assets. Back up
those settings separately and restore them from this document and the required
check manifest. Never store GitHub credentials or secrets in a repository backup
manifest.
