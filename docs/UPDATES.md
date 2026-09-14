# Settings and native updates

Settings contains Account, UI, Reporting/IMAP and API/PATs. Users, Webhooks,
Audit and Update require `ModifySystem`, including direct links. Legacy
administration URLs redirect into Settings. The notification bell is visible
only to these administrators and links to `/settings?tab=update`.

The checker reads public GitHub Releases from `darkarmy-cyber/darkphish` without
a token. It compares all three stable SemVer components, excludes drafts and
prereleases, caches automatic checks for an hour and offers Check now. Release
notes are rendered as text. A newer trusted release produces one notification.

## First-iteration apply support

Apply requires an official Linux amd64/arm64 native build, SQLite, all-mode,
a single instance, and a dedicated release directory that is also the working
directory. The executable must be named `darkphish`; `config.json` and the
SQLite file must be directly beside it. The running identity must own and be
able to write this installation. Symlinks, special files, unmanaged files,
multi-instance configurations and external database engines are unsupported.
The Update tab explains why apply is unavailable; it never falls back to a
live copy of a MySQL or PostgreSQL database.

An administrator must separately install GitHub CLI 2.100.0 or newer as
`/usr/bin/gh`. The executable and its parent directories must be root-owned,
not symlinks, and not group/world writable. Missing or outdated verifiers make
apply unavailable. Darkphish never downloads its own verifier. No GitHub login
or token is required: public attestation bundles are fetched without credentials
and verified offline against the embedded Sigstore Public Good trust root.

Keep environment files and secret mounts outside managed runtime directories.
Configured external secret/certificate files beside the binary are explicitly
excluded and listed in the backup manifest. Configured secret files may not
overlap any managed runtime entry. A custom file logger or additional runtime
files currently requires manual upgrade. After initial bootstrap, restart the
service after changing the temporary administrator password to enable the
supervisor when the bootstrap password file has been removed.

## Verification, backup and restart

Update now requests a fresh password reauthentication through the existing
privileged-session mechanism and consumes that grant. PATs cannot apply
updates. No browser-supplied download URL, shell command or filesystem path is
accepted. The supervisor independently checks the requested release again.

The trust boundary requires the canonical GitHub Actions bot identity, the
complete expected native artifact set, GitHub SHA-256 digests, SHA256SUMS,
and the publication receipt binding the checksum manifest, tag and immutable
source commit. The tag must resolve to that commit before and after download.
Before downloading or probing the replacement executable, the publication receipt
must pass Sigstore signature, certificate transparency, Rekor log and timestamp
verification. Certificate policy pins the GitHub OIDC issuer, this repository's
`release.yml@refs/heads/main`, GitHub-hosted runners, and the exact source and
signer commit. Official recovery releases instead require
`release-recover.yml@refs/heads/main` and the exact recovery execution commit,
a successful recovery publish step covering the release publication time, and
source ancestry. The signed receipt binds the released source to the checksum
manifest even when recovery executed on a later commit. A release-write
credential alone cannot forge this evidence.
The verifier receives an isolated environment/config directory without inherited
secrets or credentials. Missing attestations, unknown signing keys and every
verification failure stop apply before the application is stopped or changed.
Trust-root rotation requires a reviewed application change; it is never accepted
from update metadata. Operators must keep the independent verifier patched.
The extracted VERSION and executable build identity must agree. Archive links,
traversal, duplicate paths, unknown roots and oversized payloads are rejected.

The supervising process stops and reaps the application and all its workers
before taking a SQLite `VACUUM INTO` snapshot and checking its integrity. It
also cancels campaign scheduling and waits for active SMTP sends and their
database results to finish. It never force-kills a delivery to advance an update;
IMAP sessions have a 30-second deadline and close when polling is cancelled.
IMAP shutdown joins its manager and per-user polling goroutines. Fetch uses
BODY.PEEK; stable UIDs are marked read only after report persistence, leaving
failed or cancelled reports unread. UIDVALIDITY changes reject acknowledgements.
The supervisor
also joins accepted webhook deliveries after their event producers stop, and
archive extraction and backup copying observe service-stop cancellation. It
backs up the original config, binary, migrations, templates, static runtime
files and release metadata. Backups are owner-only, with a manifest containing
file checksums. Resolved environment secrets and Vault tokens are never
serialized. The original config is copied with any inline Vault token removed;
external secrets must be restored from the authoritative secret store.

SMTP greeting/TLS/authentication and message operations have a 30-second
deadline (connection establishment is separately bounded); QUIT is bounded to
five seconds. Worker shutdown starts alongside HTTP draining so a test-email
handler cannot keep administration waiting on a worker that has not stopped.

A completed backup is required before installation can rename any runtime
entry. A durable transaction journal supports recovery after interruption.
The state and all nested staged directories are synced before runtime mutation, and executable replacement
is atomic. Startup recovers a pending journal before new-update eligibility
checks, even when the external signature verifier is no longer available.
Recovery reads a local layout journal before loading replacement-version
configuration. Service stop signals cancel verification, readiness and apply;
an interrupted installation retains its rollback journal.
Completed recovery restores the saved transaction outcome and release tag for
status and audit reporting, including a crash before supervisor reload.
The last outcome is also persisted outside the active journal before retirement.
Only a resumed application records completion audit events, with a durable
acknowledgement to avoid replaying them on ordinary subsequent restarts.
Replacement listeners are bound but do not serve traffic until the transaction
is committed. Failed startup restores the previous application and database
snapshot, including recovery from migration changes, before traffic resumes.
Rollback failures stop the supervisor rather than starting a partial install.
Rollback restores from the checksum-verified backup without consuming it, so a
second interruption during recovery can be retried safely.
Before changing runtime files, apply preallocates rollback disk space in addition
to the complete backup and staged release. Filesystems without allocation support
or sufficient space are rejected before installation. Recovery releases that
reserve and obsolete transaction copies before restoring the verified backup.
Retained successful transaction directories include this reserve. Completion
audit acknowledgement is saved only after all persistent audit writes succeed;
an interrupted or failed write is retried on startup and may replay earlier events.
The Update tab retains backup-failure and rollback outcomes across release
checks, so the restart watcher reports what happened. Apply is also unavailable
with custom file logging or migrations outside the bundled SQLite directory.

The supervisor reloads itself after a successful update while preserving its
PID, so systemd retains the service process. Use the standard systemd
`KillMode=control-group` and a restart policy; do not run multiple services
against this SQLite installation. Update backups and results remain under
`.darkphish-updates/` for operator review and recovery. Audit events use
`update.check`, `update.backup`, `update.apply` and `update.rollback`.

No release, CI, CodeQL, branch protection or PR review gate is relaxed by this
feature. Runtime verification requires both publication metadata and independent
Sigstore evidence produced by the trusted publication workflow.
