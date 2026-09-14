---
category: Fixed
version: 0.8.0
---
- Trigger release-metadata reconciliation from either successful main CI or CodeQL completion so the later trust workflow can safely proceed once every required check for the exact protected-main SHA is green.
