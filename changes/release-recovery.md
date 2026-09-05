---
category: Changed
version: 0.3.0
---
- Make release preparation restartable with protected-main preflight, verification of generated branch history, safe fast-forward refresh, PR reuse, and explicit CI dispatch using GitHub's ephemeral token.
- Gate native publication on the protected release merge and all required checks; retain incomplete drafts for recovery, compare existing asset digests before reuse, and never overwrite release tags or binaries.
- Record standalone-repository ownership, required checks, single-maintainer protection, conservative dependency updates, and private-plan provenance limitations.
