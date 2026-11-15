---
category: Changed
version: 1.1.0
---
- Simplified the update experience: the version selector dropdown has been removed. Settings → Update now shows the current version, the latest available version, and a single "Update to [version]" button that always targets the latest stable release.
- Paginated campaign results API: `GET /api/campaigns/{id}/results?limit=50&cursor=<token>` returns one page of results (default 50, max 200) with an opaque next cursor, alongside the unchanged legacy full-detail response.
