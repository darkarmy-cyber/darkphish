---
category: Fixed
version: 0.5.0
---
- Reject permanent database trust/configuration errors immediately instead of delaying startup through transient connection retries; connection errors remain free of credentials and connection strings.
