---
category: Added
version: 1.0.0
---
- Add an optional rendered-snapshot path for Import Site so modern JavaScript-rendered pages can be captured before sanitization, with a static HTML fallback when Chromium is unavailable.
- Preserve rendered page styles and source-relative assets while continuing to strip executable scripts and force imported forms back to Darkphish.
- Bound each Chromium process tree to 512 MiB with Linux cgroup v2 and require a bounded tmpfs profile root; Windows and other deployments without an enforceable profile-storage boundary fail closed to the static HTML importer.
