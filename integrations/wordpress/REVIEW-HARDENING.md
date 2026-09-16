# WordPress licensing 0.2.4 review hardening

This candidate addresses the six findings on PR75 head `3b0fbf4` and the
four follow-up findings on `491fa8a`.

- Successful recovery invalidates every pending recovery request for the same
  normalized mailbox in the key-rotation transaction. Other mailboxes and
  registration requests are untouched. A deadlocked verification fails without
  returning a key and can be retried; two successful sibling rotations are not
  permitted.
- Failed exclusive signing-key initialization removes only the file it created,
  after checking file identity. Existing keys are never overwritten or removed.
- Loading a signing key checks both the resolved WordPress directory and the
  document root, including aliases/symlinks. A root that cannot be resolved is
  rejected. Additional web-server aliases remain the operator's responsibility.
- The shortcode script removes mailbox-verification fragments synchronously
  when loaded, before waiting for DOM readiness. Registration pages should not
  include untrusted scripts; code that executes before this script cannot be
  retroactively protected.
- Public configuration returns unavailable unless schema 4 and signing are
  ready. The shortcode uses the same readiness gate. Errors do not expose
  paths, keys or database details.
- CLI key creation also removes its own incomplete file on write, flush or
  sync failure, permits a safe retry, and never overwrites existing keys.
- If key creation succeeds but its completion audit fails, the administrator
  sees a truthful saved-key warning rather than a failed-creation message.
  The initial audit request remains mandatory before creating the file.
- Licensing SQL suppression is scoped to each operation and restored in a
  `finally` block, preserving error reporting for the rest of WordPress.
- Additive schema 4 backfills and indexes canonical email hashes separately
  from existing identity hashes. Mailbox verification uses an indexed locking
  lookup. Existing IDs, keys, bindings, terms and duplicate records are retained;
  ambiguous legacy records still require support rather than destructive merging.

Back up the database and signing key before upgrading. The schema upgrade is
retryable and does not advance its version marker on failure. No key rotation,
production migration or deployment was performed while preparing this change.
Tests use disposable data and intercepted mail. Deployment validation, final
review and release approval remain separate requirements.
