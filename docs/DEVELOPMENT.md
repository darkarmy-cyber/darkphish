# Development setup

## Prerequisites

- Go 1.27.1
- A C compiler supported by Go (SQLite uses CGO)
- Node.js 24.x
- pnpm 11.19.0 through Corepack

## Build and test

```sh
corepack enable
pnpm install --frozen-lockfile
pnpm run build
go test ./...
go test -race ./...
go vet ./...
go build -trimpath ./...
```

CI additionally runs staticcheck, govulncheck, CodeQL, pnpm audit, Trivy secret
scanning, SQLite/MySQL/PostgreSQL migration tests, MySQL/PostgreSQL core security-
model tests, and changelog-fragment validation. Generated assets under `static/*/dist` are
committed and CI verifies that a fresh frontend build does not change them.

Releases contain native binaries, checksums, an SBOM, and build provenance.

`staticcheck.conf` temporarily excludes ST1000, ST1003, and ST1005. Some public
identifiers predate those style rules, while several exported errors double as
stable user-facing API messages. They will be normalized only under an explicit
compatibility plan. All correctness, deprecation, simplification, and unused-code
checks remain enabled.

The default `config.json` is for loopback development. It intentionally allows
ephemeral session keys and plaintext integration-secret compatibility and logs a
warning for the latter. Never use those defaults for a network-exposed system.

## Tests and changes

Security and correctness fixes require a regression test demonstrating allowed,
blocked, and edge behavior. Preserve existing SQLite/MySQL data and add explicit,
reversible, database-specific migrations for persistent changes. PostgreSQL fresh
installations use the repository migration baseline. Keep simulation-page policies
separate from administrative policies.

Runtime changes require a fragment under `changes/`. Validate it with
`node scripts/changelog.mjs validate`; release preparation aggregates fragments
and advances root `VERSION` to the selected release target before creating the
protected release pull request. `VERSION` remains the authoritative SemVer source
after preparation.

Keep Darkphish naming in product code and documentation. Third-party package names,
compatibility identifiers and migration shims may remain only where technically
required. Third-party attribution belongs in `THIRD_PARTY_NOTICES.md` or the
component's own license metadata rather than product branding.

See `CONTRIBUTING.md` for conduct, licensing and disclosure guidance.
