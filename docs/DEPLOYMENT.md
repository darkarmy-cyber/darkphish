# Production and Docker deployment

The supplied image runs as UID/GID 65532, stores mutable data in `/data`, keeps
configuration at `/etc/darkphish/config.json`, and supports a read-only root
filesystem with a writable `/data` volume and `/tmp` tmpfs. The runtime image
contains only the binary and required runtime assets.

## Required production secrets

`docker/config.json` enables `production_mode`; startup therefore requires:

- `DARKPHISH_SESSION_AUTH_KEY`: exactly 32 bytes
- `DARKPHISH_SESSION_ENCRYPTION_KEY`: exactly 32 bytes
- `DARKPHISH_SECRET_ENCRYPTION_KEY`: exactly 32 bytes

The three exact-length keys accept raw text or `base64:`/`hex:` prefixes. Generate
independent random values, for example:

```sh
printf 'base64:'; openssl rand -base64 32
openssl rand -base64 48
```

Supply keys using an orchestrator secret store. File-backed configuration is
also supported through `auth_key_file`, `encryption_key_file`, and the secret
store `encryption_key_file`; paths are resolved relative to the config file.
Never commit key material.

## Runtime

```sh
docker build -t darkphish:local .
docker volume create darkphish-data
docker run --name darkphish --read-only --tmpfs /tmp \
  --volume darkphish-data:/data \
  --publish 3333:3333 --publish 8080:8080 \
  --env-file /secure/path/darkphish.env \
  darkphish:local
```

The administrative endpoint uses TLS on port 3333 and the simulation endpoint
uses port 8080 in the container configuration. Replace generated certificates
with managed certificates or terminate TLS at a trusted reverse proxy. Restrict
port 3333 to administrators. Configure scheme-qualified, exact
`trusted_origins` when a separate browser origin is required, and exact
`cors_allowed_origins` only when a separate administrative API client requires
cross-origin access. Production rejects plaintext trusted origins.

The image health check calls `https://127.0.0.1:3333/healthz`. Use `/readyz` for
traffic readiness because it also checks database connectivity.

Back up `/data`, test restoration, rotate keys through a planned maintenance
procedure, and retain the old secret-encryption key until stored integration
credentials have been re-encrypted. Losing that key makes encrypted SMTP, IMAP,
and webhook credentials unrecoverable.
