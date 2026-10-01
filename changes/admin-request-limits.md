---
category: Security
version: 0.22.3
---
- Limit administrative API requests to 100 per minute per browser user or personal token, with separate admission budgets for unverified credentials, anonymous failures and browser preflight abuse; defer API body decoding until admission, return capacity and retry information, and prevent concurrent first requests from creating independent authentication rate-limit buckets.
