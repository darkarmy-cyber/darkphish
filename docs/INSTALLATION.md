# Linux installer

Darkphish includes a production-oriented installer for a fresh native Linux deployment.
Run it only from a clean Git checkout:

```sh
git clone https://github.com/darkarmy-cyber/darkphish.git
cd darkphish
sudo ./install.sh
```

The installer currently supports amd64 and arm64 hosts using systemd and the apt,
dnf, or yum package families. It refuses upgrades or partial replacement of an
existing deployment; use the documented native update path for an installed instance.

## What the installer does

The installer:

1. verifies that it is running as root from a clean Darkphish Git checkout;
2. identifies the Linux distribution and CPU architecture;
3. installs the required compiler/runtime packages;
4. uses Go 1.27.1 when already present or downloads the official toolchain from
   `go.dev` and verifies its SHA-256 checksum before use;
5. builds Darkphish with CGO enabled and embeds the source commit/build timestamp;
6. creates the unprivileged `darkphish` system account;
7. installs the runtime below `/opt/darkphish` with the executable, database
   migrations, templates, static assets, documentation, and configuration;
8. generates independent production session, envelope-encryption, and audit-signing
   keys below `/etc/darkphish` plus a temporary self-signed administrative TLS certificate;
9. configures SQLite, production mode, and the owner-only initial administrator
   password output directory;
10. creates and validates a hardened `darkphish.service` unit and starts it.

The application does not run as root. The systemd unit grants only
`CAP_NET_BIND_SERVICE`, allowing the simulation listener to use TCP/80 while the
process remains the dedicated `darkphish` account.

## Installed layout

- `/opt/darkphish` - native runtime and writable SQLite database
- `/opt/darkphish/config.json` - production configuration
- `/etc/darkphish` - root-managed production key material and administrative TLS files
- `/var/lib/darkphish/bootstrap` - owner-only initial administrator password output
- `/etc/systemd/system/darkphish.service` - native systemd service

The runtime directory is owned by `darkphish` because the verified native updater
requires ownership of the managed single-instance SQLite layout. Production key
material remains root-owned and group-readable only by the Darkphish service account.

## First login

The administration listener defaults to loopback only:

```text
https://127.0.0.1:3333
```

The administrator username is `admin`. Read the generated initial password locally:

```sh
sudo cat /var/lib/darkphish/bootstrap/darkphish_initial_admin_password
```

For remote administration, keep the listener on loopback and tunnel it over SSH:

```sh
ssh -L 3333:127.0.0.1:3333 user@server
```

Then open `https://localhost:3333`, accept the temporary self-signed certificate,
sign in, and change the bootstrap password immediately. Darkphish removes the
bootstrap password file after the required password change.

Before production exposure, replace the temporary administrative certificate or
terminate TLS through an approved reverse proxy, configure the intended DNS/listener
policy, review `docs/DEPLOYMENT.md`, and test backups/restores.

## Service operations

```sh
systemctl status darkphish
journalctl -u darkphish -f
systemctl restart darkphish
```

The installer does not automatically install GitHub CLI. The optional in-product
verified native update feature requires a separately installed root-owned
`/usr/bin/gh` version 2.100.0 or newer; see `docs/UPDATES.md`.

## Safety and repeatability

The installer intentionally fails closed when it finds an existing Darkphish
service, user/group, runtime/configuration/state directory, unsupported platform,
dirty Git checkout, symbolic links in the managed runtime payload, malformed Go
checksum response, failed checksum verification, failed build, invalid systemd
unit, or a service that does not become active.

It is therefore not an unattended upgrade mechanism. Re-running it against an
installed host is expected to stop with an error rather than mutate an existing
instance.
