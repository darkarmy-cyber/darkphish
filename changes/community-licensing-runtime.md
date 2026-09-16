---
category: Security
version: 0.11.0
---
- Enforce Community licensing at server startup, serialize entitlement checks across database writers, reject caller-supplied creation IDs, and recheck license validity before queued campaign delivery.
- Reject licensing redirects and incomplete activation responses, preserve existing recipient identities in degraded mode, and add administrator activation controls with periodic signed-lease refresh.
- Distribute the maintainer-provided fsociety.sk public verification key and API configuration with native archives, the source installer and Docker, retaining license state on persistent storage.
- Accept authenticated Professional and Enterprise edition claims with the same installation, expiry and signed entitlement enforcement; reject unknown editions.
