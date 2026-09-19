---
category: Fixed
version: 0.17.0
---
- Block campaign creation and every test-email entry point without a valid activated license, with clear activation guidance and disabled sending controls.
- Recheck licensing before SMTP connections and each message, retaining unsent campaign recipients for retry after license restoration.
