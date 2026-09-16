---
category: Security
version: 0.11.0
---
- Create Windows licensing state with a protected owner-only DACL before writing credentials, including every atomic replacement, so shared-directory permissions cannot expose refresh tokens. Unix file-mode protections remain unchanged.
