# Darkphish

![Darkphish](static/images/darkphish_banner.png)

[![CI](https://github.com/darkarmy-cyber/darkphish/actions/workflows/ci.yml/badge.svg)](https://github.com/darkarmy-cyber/darkphish/actions/workflows/ci.yml)
[![CodeQL](https://github.com/darkarmy-cyber/darkphish/actions/workflows/codeql.yml/badge.svg)](https://github.com/darkarmy-cyber/darkphish/actions/workflows/codeql.yml)

Darkphish is an open-source platform for authorized phishing simulations,
security-awareness training, internal security testing, and defensive research.
It is derived from Gophish and keeps its straightforward single-binary design
while establishing a secure, actively maintained foundation.

Use Darkphish only where you have explicit authorization. It is not intended
for credential theft, malware delivery, security-control evasion, or targeting
third parties.

## Build from source

Requirements:

- Go 1.27.1 (CGO and a C compiler are required for SQLite)
- Node.js 24.x and pnpm 11.19.0 for frontend changes

```sh
go test ./...
go build -trimpath -o darkphish ./
pnpm install --frozen-lockfile
pnpm run build
```

Start the development configuration with:

```sh
./darkphish --config config.json
```

The initial administrator password is written with owner-only permissions to
`darkphish_initial_admin_password` beside the SQLite database. The password is
never printed to logs and the file is removed after the required first password
change. You can instead set `DARKPHISH_INITIAL_ADMIN_PASSWORD` or configure
`DARKPHISH_INITIAL_ADMIN_PASSWORD_FILE`.

Development defaults are deliberately separate from production. Read
[development setup](docs/DEVELOPMENT.md) and [production deployment](docs/DEPLOYMENT.md)
before exposing an instance to a network.

## Deployment

Native binaries are the supported release artifacts. Historical container
files remain optional community tooling, but container builds and publishing
are not required CI or release gates. See [deployment](docs/DEPLOYMENT.md) for
key generation, persistence, TLS, database, and health-check guidance.

## Security baseline

- Browser clients use encrypted, signed, `HttpOnly`, `SameSite=Lax` sessions.
- External API clients authenticate with scoped, expiring personal access tokens
  sent as `Authorization: Bearer <token>`.
- API tokens in query strings are rejected.
- Administrative CORS is off unless exact trusted origins are configured.
- Production mode requires persistent session and secret-encryption keys.
- Stored SMTP, IMAP, and webhook secrets are write-only in normal API responses.
- Campaigns default to disabled credential handling. Policy-only mode stores
  irreversible findings; encrypted review requires explicit authorization,
  short retention, and an individually audited reveal.
- Administrative changes and sensitive reads emit persistent, indexed,
  secret-free audit events.

Raw personal access tokens are displayed once and never stored. Legacy
permanent API keys no longer authenticate.

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Development](docs/DEVELOPMENT.md)
- [Version and release policy](docs/RELEASES.md)
- [Production deployment](docs/DEPLOYMENT.md)
- [Credential review](docs/CREDENTIAL_REVIEW.md)
- [Personal access tokens](docs/API_TOKENS.md)
- [Migration from the upstream baseline and 0.2](docs/MIGRATION.md)
- [Security policy](SECURITY.md)
- [Changelog](CHANGELOG.md)

## License and origin

Darkphish is MIT licensed. The original copyright and permission notice are
preserved in [LICENSE](LICENSE). See [NOTICE.md](NOTICE.md) for derivation and
attribution details.
