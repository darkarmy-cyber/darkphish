---
category: Security
version: 0.22.3
---
- Limit administrative API requests to 100 per minute per browser user or personal token, with separate admission budgets for unverified credentials, anonymous failures and browser preflight abuse; defer API body decoding until admission, return capacity and retry information, and prevent concurrent first requests from creating independent authentication rate-limit buckets.
- Limit personal token creation to five requests per minute per user and 100 active tokens, and update admission recognition incrementally so creating tokens cannot force repeated full database scans or obtain unlimited fresh creation capacity.
