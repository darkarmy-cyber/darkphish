---
category: Fixed
version: 0.10.0
---
- Harden the native Linux installer against inherited command-bearing environments, require systemd 245+ with a reachable manager, reject unusable `noexec` build locations before persistent mutation, and require stable readiness from both the administrative and simulation listeners before reporting success.
