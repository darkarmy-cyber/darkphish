---
category: Fixed
version: 0.29.0
---
- Rendered Import Site waits longer for Chromium to start. On a slow host a cold browser needed more than the five seconds allowed under the renderer's CPU limit, and the import fell back to the static response with "cdp stage failed: port" in the log. The wait is now 15 seconds within the unchanged 25-second import budget.
