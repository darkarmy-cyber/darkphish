---
category: Fixed
version: 0.28.3
---
- Rendered Import Site works again with Chromium 158 and later (stable since 17 November 2026). The renderer was allowed 128 processes and threads; current Chromium holds 140 threads before it receives the first command, so it aborted at once and every rendered import silently fell back to the static HTML response. The limit is now 512, more than twice the 221 threads measured while rendering YouTube, the Google sign-in page, BBC and GitHub. The 512 MiB memory cap per render is unchanged.
