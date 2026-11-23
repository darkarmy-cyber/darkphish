---
category: Changed
version: 0.29.0
---
- Releases are checked with a real Chromium. A new smoke test renders two public pages through the renderer on pull requests that change it, every Monday and on demand (`scripts/render-smoke.sh` does the same on a Linux host). The unit suite never starts a browser, which is how a renderer limit that aborted Chromium 158 reached the 0.28.0 to 0.28.2 releases unnoticed. The application itself is unchanged.
