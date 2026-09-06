---
category: Migration
version: 0.5.0
---
- Existing v0.4 integration settings may contain plaintext left by automatic association saves. GORM v2 campaign persistence prevents new association rewrites; inspect existing data with `secrets status` and explicitly repair it with `secrets migrate` after backing up and configuring the key provider. Startup preserves existing bytes and does not silently rotate secrets.
