---
category: Fixed
version: 0.3.0
---
- Non-retainable repeat credential submissions now atomically purge any older ciphertext for the same result, preventing stale plaintext from remaining revealable.
