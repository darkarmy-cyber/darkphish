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

The installer currently supports amd64 and arm64 hosts using systemd 245 or newer and
the apt, dnf, or yum package families. systemd 245 is the minimum because the hardened
unit requires the complete configured sandbox, including `ProtectClock=`, in addition to
`AmbientCapabilities=CAP_NET_BIND_SERVICE`. Older managers are refused before package
installation or other persistent host mutation. The installer also verifies connectivity
to the running systemd manager rather than relying only on the client binary or
`/run/systemd/system` directory.

It refuses upgrades or partial replacement of an existing deployment; use the documented
native update path for an installed instance.

## What the installer does

The installer:

1. verifies that it is running as root from a valid, clean DarkPhish Git checkout;
2. performs privileged Git inspection in a clean environment with replacement objects
   disabled and repository filesystem monitors disabled, so local Git metadata cannot
   substitute source objects or execute a checkout-controlled helper during verification;
3. refuses existing runtime paths, dangling links, systemd units and drop-in overrides;
4. identifies the Linux distribution and CPU architecture, verifies connectivity to the
   running systemd manager, and requires systemd 245+;
5. creates a private temporary build directory only on a filesystem where root can execute
   files, refusing hardened `noexec` temporary layouts that cannot support the verified
   toolchain/build workflow;
6. creates an immutable source snapshot from the exact Git commit using a sanitized tar
   environment so inherited `TAR_OPTIONS` cannot inject privileged extraction actions;
7. installs the required compiler/runtime packages;
8. uses Go 1.27.1 when already present or downloads the official toolchain from
   `go.dev` and verifies it against a reviewed SHA-256 digest pinned inside the installer;
9. builds only the tracked source snapshot in an empty allowlisted environment. External
   Go workspaces, user Go configuration, inherited `GOROOT`, `GOAUTH`, CGO flags, private
   module overrides, and other caller-controlled Go environment state are not inherited;
10. creates the unprivileged `darkphish` system account;
11. installs the runtime below `/opt/darkphish` with the executable, database
    migrations, templates, static assets, documentation, and configuration;
12. generates independent production session, envelope-encryption, and audit-signing
    keys below `/etc/darkphish` plus a temporary self-signed administrative TLS certificate;
13. configures SQLite, production mode, and the owner-only initial administrator
    password output directory;
14. creates and validates a hardened `darkphish.service`, verifies the loaded unit has
    no unexpected drop-ins, starts it, and requires stable readiness from both the
    administrative listener and the TCP/80 simulation listener plus bootstrap-password
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
systemd drop-in, unsupported platform or systemd version, unreachable systemd manager,
unverified Git state, symbolic links in the managed runtime payload, an unusable
`noexec` build location, failed pinned Go checksum verification, failed build,
unexpected loaded systemd unit, failed administrative readiness, failed simulation
listener readiness, or missing bootstrap output.

Privileged Git, tar, OpenSSL, Go, and built-binary execution paths use sanitized or
allowlisted environments so caller-controlled command-bearing variables are not carried
through a sudo boundary.

It is not an unattended upgrade mechanism. Re-running it against an installed host
is expected to stop with an error rather than mutate an existing instance.
