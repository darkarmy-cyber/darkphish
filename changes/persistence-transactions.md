---
category: Security
version: 0.5.0
---
- Preserve security transaction rollback on outbox failure and harden panic/commit handling; retain unscoped-write protection with explicit join-table inserts and scoped mail-lock updates.
- Extend the shared database contract with nested transactions, cancellation, failed PAT/reviewer/account changes, ciphertext-write failure, secret-rotation rollback, summary ownership and unchanged campaign-association checks.
