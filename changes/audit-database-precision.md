---
category: Fixed
version: 0.3.0
---
- Compute audit hashes from the persisted database representation and sign checkpoint timestamps at portable microsecond precision so MySQL/PostgreSQL timestamp rounding cannot invalidate an untampered chain.
