---
category: Fixed
version: 0.3.0
---
- Store absent user/IMAP last-login and campaign completion/send-by timestamps as SQL NULL across SQLite, strict MySQL, and PostgreSQL; show no last-login date for accounts that have never authenticated.
