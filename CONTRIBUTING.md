# Contribute to Darkphish

Thank you for your interest in contributing to Darkphish.

Darkphish Community is the public, free and open-source edition of the project. It is maintained as a genuinely useful platform for authorized phishing simulations, security-awareness work and defensive research. Commercial Professional, Enterprise and MSP capabilities, if offered, are developed separately and do not change the open-source status of this repository.

## Before contributing

- Use Darkphish only for explicitly authorized security-awareness and defensive testing.
- Do not submit features for credential replay, password spraying, credential stuffing, captured-password validation against third-party systems, mail-filter bypass, anti-sandbox behavior, anti-detection or defensive evasion.
- Never include real credentials, tokens, customer data, production configuration, DSNs or personal information in issues, pull requests, tests or logs.
- Keep changes focused, reviewable and covered by appropriate tests.

## Contributor license agreement

By submitting code as an individual you agree to the
[individual contributor license agreement](doc/individual_contributor_license_agreement.md).
By submitting code as an entity you agree to the
[corporate contributor license agreement](doc/corporate_contributor_license_agreement.md).

## Security vulnerability disclosure

Report suspected vulnerabilities privately through the
[GitHub Security Advisory form](https://github.com/darkarmy-cyber/darkphish/security/advisories/new).
Do **not** disclose exploitable details in a public issue.

## Issues and pull requests

Before filing an issue, search existing issues and review the repository documentation. Bug reports should include the affected Darkphish version or commit, expected behavior, actual behavior and minimal reproduction steps using synthetic data.

Issues and pull requests should be written in English and remain professional and suitable for a public security project. Requests outside the project's authorization and safety boundaries may be closed without implementation.

Pull requests should:

- target the current public Community codebase;
- avoid unrelated refactors;
- preserve backwards compatibility unless a breaking change is explicitly justified;
- include tests for behavior changes where practical;
- pass the repository's required CI and security checks;
- follow the security and release policies documented in this repository.

## Getting started

If you want to contribute but are not sure where to begin, look for issues labelled `contributor-friendly`:
https://github.com/darkarmy-cyber/darkphish/labels/contributor-friendly

For usage and development guidance, start with [README.md](README.md), [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md), [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md), and [SECURITY.md](SECURITY.md).
