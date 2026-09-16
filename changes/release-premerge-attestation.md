---
category: Security
version: 0.10.0
---
- Require the existing exact-head maintainer release attestation before a generated release can merge, and revalidate it immediately before the protected merge. Missing, stale, forged, revoked or unresolved approval now stops the merge instead of stranding publication afterward.
