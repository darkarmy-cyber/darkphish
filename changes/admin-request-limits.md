---
category: Security
version: 0.22.3
---
- Limit administrative API requests to 100 per minute per verified user, with separate client-IP budgets for anonymous failures and browser preflight abuse; defer API body decoding until admission, return capacity and retry information, and prevent concurrent first requests from creating independent authentication rate-limit buckets.
