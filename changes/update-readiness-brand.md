---
category: Fixed
version: 0.15.0
---
- Explain disabled one-click updates separately from release notifications, with verifier prerequisites and service-restart guidance. Recheck server state after failed update confirmation before allowing a retry, without repeating the apply request or weakening verification.
- Use the official DarkPhish spelling consistently in application pages, notifications, command-line messages and maintained documentation while preserving technical identifiers and historical release records.
- Include the official bundled public license keyring in the native updater's validated runtime, verified backup, installation and rollback. Existing affected updaters require a verified manual upgrade to receive this fix; do not delete the keyring or bypass eligibility checks.
