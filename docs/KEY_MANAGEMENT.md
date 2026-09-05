# Envelope key management

Darkphish separates three key purposes: browser-session authentication/encryption,
credential and integration-secret envelope encryption, and audit integrity. Never
reuse one key for another purpose.

V3 records use AES-256-GCM with a fresh random data-encryption key (DEK). The active
provider wraps only that DEK. The stored envelope records its version, provider,
key reference, wrapped DEK, nonce, ciphertext, algorithm, and creation time. The
provider and key reference are authenticated as associated data. Unknown providers,
wrong references, corrupt wrapped keys/ciphertext, and provider outages fail closed.

The `local` provider keeps 0.2 v2 key-ID envelopes and older v1 ciphertext readable
while their keys are configured. The `vault` provider uses Vault Transit encrypt
and decrypt endpoints over an HTTPS client with TLS 1.2 minimum, an optional private
CA, a ten-second timeout, bounded responses, and sanitized errors. Supply its token
through `DARKPHISH_VAULT_TOKEN` or a protected file and grant only the selected
Transit key's encrypt/decrypt capabilities.

## Rotation and migration

1. Configure the old read key/provider and the new active provider/key together.
2. Run `darkphish secrets status`; it reports legacy, active-local, external,
   plaintext, provider/key counts, and records requiring migration without values.
3. Run `darkphish secrets migrate`. Each SMTP, IMAP, webhook, and retained
   credential record is opened and re-sealed individually. The row update and a
   secret-free audit-outbox event commit together. Completed rows are not rewritten
   on a retry.
4. Rerun status and application integration checks. Confirm no records or retained
   backups still require the old key before removing it.

Migration does not write plaintext files and logs only table/row identifiers on an
operator-visible failure. Provider errors are never replaced by a silent local-key
fallback. Losing required local keys or the external provider key makes the affected
ciphertext unrecoverable.

Back up local keys and audit seeds in the organization's secret manager, separately
from database dumps. For Vault, protect policies, mount/key identity, recovery keys,
and the ability to restore access to the same Transit ciphertext versions. A restored
database must pass `secrets status`, synthetic integration checks, and audit
verification before use.
