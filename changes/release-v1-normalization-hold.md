---
category: Fixed
version: 1.0.0
---
- Hold ambiguous `v1.0.0` recovery attempts at the reviewed merged release source so scheduled reconciliation exits cleanly without publishing or sending repeated failure notifications.
- Update the Go toolchain to 1.27.2 and `golang.org/x/net` to 0.60.0 to remediate newly disclosed standard-library and HTTP/2 vulnerabilities.
