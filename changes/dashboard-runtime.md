---
category: Fixed
version: 0.22.0
---
- Include the v0.22 dashboard KPI controller in production Docker images so operational metrics render outside source checkouts.
- Refresh KPI values when summary statistics change and reset them for empty results, with browser coverage across mobile, tablet and desktop layouts.
- Add an explicit analytics refresh that fetches a new authorized summary and updates KPI cards, charts and the campaign table together.
