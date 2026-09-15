# Linux installer

DarkPhish includes a production-oriented installer for fresh native Linux deployments.
For production, install from an immutable published release tag rather than the moving
`main` branch. Starting with v0.10.0:

```sh
git clone --branch v0.10.0 --depth 1 https://github.com/darkarmy-cyber/darkphish.git
cd darkphish
sudo ./install.sh
```

For later releases, replace `v0.10.0` with the release tag you intend to deploy.
An untagged checkout can still be built deliberately, but DarkPhish marks it as a
development build and verified in-product updates remain unavailable.

The installer currently supports amd64 and arm64 hosts using systemd and the apt,
dnf, or yum package families. It refuses upgrades or partial replacement of an
existing deployment; use the documented native update path for an installed instance.

## What the installer does

The installer:

1. verifies that it is running as root from a valid, clean DarkPhish Git checkout;
2. refuses existing runtime paths, dangling links, systemd units and drop-in overrides;
3. identifies the Linux distribution and CPU architecture;
4. installs the required compiler/runtime packages;
5. uses Go 1.27.1 when already present or downloads the official toolchain from
   `go.dev` and verifies it against a reviewed SHA-256 digest pinned inside the installer;
6. creates an immutable source snapshot from the exact Git commit and builds only that
   tracked content with external Go workspaces and user Go environment disabled;
7. creates the unprivileged `darkphish` system account;
8. installs the runtime below `/opt/darkphish` with the executable, database
   migrations, templates, static assets, documentation, and configuration;
9. generates independent production session, envelope-encryption, and audit-signing
   keys below `/etc/darkphish` plus a temporary self-signed administrative TLS certificate;
10. configures SQLite, production mode, and the owner-only initial administrator
    password output directory;
11. creates and validates a hardened `darkphish.service`, verifies the loaded unit has
    no unexpected drop-ins, starts it, and waits for `/readyz` plus bootstrap-password
    creation before reporting success.

The application does not run as root. The systemd unit grants only
`CAP_NET_BIND_SERVICE`, allowing the simulation listener to use TCP/80 while the
process remains the dedicated `darkphish` account.

If installation fails after host mutation begins, the installer stops/disables any
unit it created and removes only the DarkPhish account, service file, and directories
that this fresh-install transaction created. System packages installed as prerequisites
are not removed.

## Installed layout

- `/opt/darkphish` - native runtime and writable SQLite database
- `/opt/darkphish/config.json` - production configuration
- `/etc/darkphish` - root-managed production key material and administrative TLS files
- `/var/lib/darkphish/bootstrap` - owner-only initial administrator password output
- `/etc/systemd/system/darkphish.service` - native systemd service

The runtime directory is owned by `darkphish` because the verified native updater
requires ownership of the managed single-instance SQLite layout. Production key
material remains root-owned and group-readable only by the DarkPhish service account.

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
sign in, and change the bootstrap password immediately. DarkPhish removes the
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

The installer intentionally fails closed when it finds an existing DarkPhish
service, user/group, runtime/configuration/state directory, dangling managed path,
systemd drop-in, unsupported platform, unverified Git state, symbolic links in the
managed runtime payload, failed pinned Go checksum verification, failed build,
unexpected loaded systemd unit, failed application readiness, or missing bootstrap
output.

It is not an unattended upgrade mechanism. Re-running it against an installed host
is expected to stop with an error rather than mutate an existing instance.
