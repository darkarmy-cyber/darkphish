---
category: Changed
version: 0.3.0
---
- Sensitive multi-write operations now use focused repositories, service boundaries, database transactions, and a durable audit outbox so partial credential, reviewer, token, user-role, or audit state is rolled back safely.
