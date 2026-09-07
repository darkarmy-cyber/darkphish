---
category: Security
version: 0.8.0
---
- Require authenticated, completed code and explicit security reviews of the current head and resolved review threads before automated engineering or generated-release merges; fail closed on stale, edited, ambiguous or unavailable evidence.
- Replace queued merge permissions with a single protected squash merge matching the complete head SHA, preserve label-removal pauses during release recovery, and recheck pre-merge review evidence before native publication.
