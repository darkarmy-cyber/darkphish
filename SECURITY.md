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
all required persistent keys from a secret manager or mounted files, restrict
network access, back up application and database data, and keep the host patched. The
simulation listener is a separate trust boundary and should not expose the
administrative interface.

Keep audit exports and individually revealed credentials within the authorized
review group. Never place revealed values in tickets, chat, logs, webhooks, or
bulk exports. Encrypted review is an exceptional campaign mode, not a default.

Darkphish is for explicitly authorized simulations only. Vulnerability reports
must not use third-party systems or real employee credentials as test material.
