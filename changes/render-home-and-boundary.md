---
category: Fixed
version: 0.28.1
---
- Rendered Import Site no longer lets Chromium write cache directories into the service account's home directory. On native installations that home is the installation directory, so the first rendered import added `.cache`, `.config` and `.local` there and one-click updates refused to run with `unmanaged runtime files`. Chromium now keeps everything outside its profile inside the bounded, temporary profile directory.
- Removed the unused Entra ID group preview source left in the free edition after the paid directory code was removed. It had no callers; the free edition ships no directory-sync implementation.
