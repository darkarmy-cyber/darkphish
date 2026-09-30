---
category: Fixed
version: 0.22.2
---
- Reduce update catalogue checks to the release listing request by reading the canonical immutable source marker from trusted release notes, avoiding one GitHub API tag lookup per historical release and preventing avoidable HTTP 403 rate-limit failures.
