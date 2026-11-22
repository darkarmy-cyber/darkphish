---
category: Security
version: 0.28.3
---
- Rendered Import Site no longer hands the DarkPhish service environment to Chromium. The renderer received every variable of the service, including secrets configured through the environment (`DARKPHISH_SECRET_*`, `DARKPHISH_VAULT_TOKEN`, `DARKPHISH_POSTGRES_PASSWORD`, `DARKPHISH_AUDIT_SIGNING_KEY*`), on installations that run Chromium without a sandbox. It now receives only `PATH`, the locale and time zone variables, the temp location, `LD_LIBRARY_PATH` and `CHROME_DEVEL_SANDBOX`; its home directories stay inside the render profile as before.
