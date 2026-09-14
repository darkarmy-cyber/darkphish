---
category: Fixed
version: 0.8.0
---
- Treat a withdrawn draft whose GitHub-generated URL is already `untagged-*` as detached during trusted tag repair even if the release-list `tag_name` field is stale, while retaining fail-closed checks for real tag conflicts.
