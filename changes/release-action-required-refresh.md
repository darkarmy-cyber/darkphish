---
category: Fixed
version: 0.7.1
---
- Recover generated release checks that GitHub marks `action_required` before any job executes, while preserving fail-closed behavior for real CI or CodeQL failures and bounding each exact-head recovery dispatch to a single attempt.
