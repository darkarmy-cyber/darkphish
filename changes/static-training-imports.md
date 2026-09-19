---
category: Security
version: 0.20.0
---
- Make new site imports inert, visibly labelled training pages: remove active forms, scripts and field values; persist the restricted mode through edits and render literal text under a sandboxed policy. Existing legacy pages are not automatically converted.
- Embed explicitly requested, verified raster images in static training pages without relaxing destination or administrative browser protections. Dynamic stylesheets, executable content and unsupported image formats remain unavailable.
- Remove the redundant Workspace heading, give import actions a distinct high-contrast teal style, and clarify SMTP authentication failures without changing sending behavior.
