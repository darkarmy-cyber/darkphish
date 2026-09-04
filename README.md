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

## Docker

```sh
docker build -t darkphish:local .
docker run --rm -p 3333:3333 -p 8080:8080 \
  --read-only --tmpfs /tmp --volume darkphish-data:/data \
  --env DARKPHISH_SESSION_AUTH_KEY='base64:<32-byte-key>' \
  --env DARKPHISH_SESSION_ENCRYPTION_KEY='base64:<32-byte-key>' \
  --env DARKPHISH_SECRET_ENCRYPTION_KEY='base64:<32-byte-key>' \
  darkphish:local
```

See [Docker deployment](docs/DEPLOYMENT.md) for key generation, persistence,
TLS, reverse-proxy, and health-check guidance.

## Security baseline

- Browser clients use encrypted, signed, `HttpOnly`, `SameSite=Lax` sessions.
- External API clients authenticate with `Authorization: Bearer <token>`.
- API tokens in query strings are rejected.
- Administrative CORS is off unless exact trusted origins are configured.
- Production mode requires persistent session and secret-encryption keys.
- Stored SMTP, IMAP, and webhook secrets are write-only in normal API responses.
- Credential submissions retain field names only; submitted values are discarded.
- Administrative changes and sensitive reads emit structured, secret-free audit events.

Long-lived API tokens are no longer placed in browser HTML or JavaScript. The
current compatibility token is returned only immediately after rotation. A
hashed, named, scoped, expiring personal-access-token model remains planned.

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Development](docs/DEVELOPMENT.md)
- [Production and Docker deployment](docs/DEPLOYMENT.md)
- [Migration from the upstream baseline](docs/MIGRATION.md)
- [Security policy](SECURITY.md)
- [Changelog](CHANGELOG.md)

## License and origin

Darkphish is MIT licensed. The original copyright and permission notice are
preserved in [LICENSE](LICENSE). See [NOTICE.md](NOTICE.md) for derivation and
attribution details.
