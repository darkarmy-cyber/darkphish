# Security policy

## Reporting a vulnerability

Please report suspected vulnerabilities privately through the
[Darkphish GitHub security advisory form](https://github.com/darkarmy-cyber/darkphish/security/advisories/new).
Do not disclose exploitable details in a public issue.

Include the affected version or commit, deployment model, reproduction steps,
impact, and any suggested mitigation. Never include real credentials, campaign
data, or personal information. The Darkphish maintainers will acknowledge the
report, investigate it, coordinate a fix and disclosure date, and credit the
reporter when requested.

Reports concerning unmodified upstream code are still welcome here: Darkphish
maintainers own the security response for Darkphish deployments.

## Supported versions

Until Darkphish reaches 1.0, security fixes are made on the current development
branch and latest tagged minor release. Operators should track the newest
security release; older pre-1.0 versions do not receive indefinite backports.

## Deployment expectations

Run the administrative server behind TLS, enable `production_mode`, provide
independent persistent session, envelope, and Ed25519 audit-signing keys from a
secret manager or mounted files, restrict network access, back up application and
database data, and keep the host patched. The
simulation listener is a separate trust boundary and should not expose the
administrative interface.

Keep signed audit exports and individually revealed credentials within the
authorized review group. Never place revealed values in tickets, chat, logs,
webhooks, analytics, or bulk exports. Encrypted review is an exceptional campaign
mode, not a default. Require named, expiring campaign assignments for Security
Reviewers and investigate `credential.view`, reauthentication failure, and reviewer
lifecycle events.

Use PostgreSQL `verify-full` TLS with a trusted server identity in production.
Vault Transit deployments require least-privilege access to one named key and must
fail closed when Vault is unavailable. Database backups do not replace key backups:
without the required provider/local keys they cannot restore encrypted values, and
without audit verification material they cannot establish checkpoint authenticity.

Darkphish is for explicitly authorized simulations only. Vulnerability reports
must not use third-party systems or real employee credentials as test material.
