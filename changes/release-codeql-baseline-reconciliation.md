---
category: Security
version: 0.7.1
---
- Reconcile trusted generated-release full-head CodeQL results only against the unchanged protected-main baseline for generated-only release PRs, preserving local SARIF evidence while continuing to fail closed on unresolved alerts, unexpected files, or target/base changes.
