---
category: Changed
version: 0.7.0
---
- Read audit event and checkpoint history in bounded keyset pages during startup, chain verification, retention, and signed export construction, preserving transactional chain coordination and existing hashes, signatures, and export bytes.
- Exercise page-boundary corruption, rollback, typed retry, legacy receipt backfill, and multi-page MySQL/PostgreSQL multi-process history without weakening audit verification. Signed export payloads remain buffered by the existing byte-array interface; full streaming export and shorter writer-lock duration remain future work.
