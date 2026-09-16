# WordPress licensing 0.2.5 review hardening

This candidate addresses the six findings on PR75 head `3b0fbf4` and the
four follow-up findings on `491fa8a`, plus header timing and cleanup backlog
and public signing-path alias findings on `b6d93fe`.

- Successful recovery invalidates every pending recovery request for the same
  normalized mailbox in the key-rotation transaction. Other mailboxes and
  registration requests are untouched. A deadlocked verification fails without
  returning a key and can be retried; two successful sibling rotations are not
  permitted.
- Failed exclusive signing-key initialization removes only the file it created,
  after checking file identity. Existing keys are never overwritten or removed.
- Loading a signing key checks both the resolved WordPress directory and the
  document root. Both key loading and admin initialization reject any symlink
  component, relative path or dot segment before resolving the configured
  pathname, so a public symlink cannot disappear into its private target.
  A public root that cannot be resolved is rejected. Additional web-server
  aliases or hardlinks unrelated to the configured path remain the operator's
  responsibility; do not expose the private directory through server aliases.
- The shortcode script is loaded as a parser-blocking same-origin asset at the
  earliest `wp_head` priority, compatible with CSP `script-src 'self'`,
  before WordPress-enqueued theme, plugin and analytics scripts. It removes
  mailbox-verification fragments synchronously and keeps the token in a closure
  until explicit confirmation. Themes must call `wp_head` before their own raw
  scripts; registration pages must not include untrusted scripts. Scripts that
  a theme hardcodes before `wp_head` cannot be retroactively protected. Exclude
  this bootstrap from script optimizers that add async/defer or relocate scripts.
- Public configuration returns unavailable unless schema 4 and signing are
  ready. The shortcode uses the same readiness gate. Errors do not expose
  paths, keys or database details.
- CLI key creation also removes its own incomplete file on write, flush or
  sync failure, permits a safe retry, and never overwrites existing keys. It
  validates the direct absolute destination before creating any file, even when
  the operator's working directory is publicly served.
- If key creation succeeds but its completion audit fails, the administrator
  sees a truthful saved-key warning rather than a failed-creation message.
  The initial audit request remains mandatory before creating the file.
- Licensing SQL suppression is scoped to each operation and restored in a
  `finally` block, preserving error reporting for the rest of WordPress.
- Cleanup drains up to ten 1,000-row batches per table per invocation and
  schedules a separate one-minute continuation when the bounded budget is
  exhausted. Hourly scheduling no longer limits cleanup to 1,000 rows per hour;
  production sites should invoke WordPress cron regularly even without traffic.
- Additive schema 4 backfills and indexes canonical email hashes separately
  from existing identity hashes. Mailbox verification uses an indexed locking
  lookup. Existing IDs, keys, bindings, terms and duplicate records are retained;
  ambiguous legacy records still require support rather than destructive merging.

Back up the database and signing key before upgrading. The schema upgrade is
retryable and does not advance its version marker on failure. No key rotation,
production migration or deployment was performed while preparing this change.
Tests use disposable data and intercepted mail. Deployment validation, final
review and release approval remain separate requirements.
