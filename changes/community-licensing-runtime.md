---
category: Security
version: 0.11.0
---
- Enforce Community licensing at server startup, serialize entitlement checks across database writers, reject caller-supplied creation IDs, and recheck license validity before queued campaign delivery.
- Reject licensing redirects and incomplete activation responses, preserve existing recipient identities in degraded mode, and add administrator activation controls with periodic signed-lease refresh.
