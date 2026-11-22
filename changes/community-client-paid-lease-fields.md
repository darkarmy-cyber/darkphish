---
category: Fixed
version: 0.28.2
---
- The Community edition again activates and refreshes licenses issued for Professional and Enterprise editions. Since the licensing service began advertising the latest paid version, its responses for those licenses carried a `latest_paid_version` field that the Community client rejected, so the lease was never renewed and would have expired on its signed date. The same release repairs it without operator action.
- The licensing client now states which step failed when a lease refresh is rejected (service HTTP status, response validation or lease verification). It previously logged only that the refresh had failed.
