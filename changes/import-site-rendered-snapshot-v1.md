---
category: Added
version: 1.0.0
---
- Add an optional rendered-snapshot path for Import Site so modern JavaScript-rendered pages can be captured before sanitization, with a static HTML fallback when Chromium is unavailable.
- Preserve rendered page styles and source-relative assets while continuing to strip executable scripts and force imported forms back to Darkphish.
- Bound each Chromium process tree to 512 MiB with a Linux cgroup v2 or Windows Job Object; deployments without an enforceable memory boundary fail closed to the static HTML importer.
