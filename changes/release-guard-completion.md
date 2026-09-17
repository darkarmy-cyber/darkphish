---
category: Fixed
version: 0.13.0
---
- Revalidate immutable recovery execution before every tagless-release withdrawal and retry, stopping stale operations without changing release visibility.
- Preserve nanosecond review chronology and compare summary metadata at its actual reported decimal precision.
