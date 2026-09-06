---
category: Fixed
version: 0.5.0
---
- Validate configured PostgreSQL CA files before opening the database, so unreadable or malformed trust files fail immediately with a redacted configuration error instead of transient connection retries.
