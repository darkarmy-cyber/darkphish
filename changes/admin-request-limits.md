---
category: Security
version: 0.22.3
---
- Limit administrative API requests to 100 per minute per client IP, return capacity and retry information, and prevent concurrent first requests from creating independent authentication rate-limit buckets.
