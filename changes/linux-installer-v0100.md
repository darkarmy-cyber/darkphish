---
category: Added
version: 0.10.0
---
- Add a hardened `./install.sh` for fresh systemd-based Linux deployments that verifies the host and pinned Go toolchain, builds only the exact tracked source snapshot, creates a dedicated service account, generates production security material, installs the native runtime, validates application readiness, and rolls back installer-created artifacts on failure.
