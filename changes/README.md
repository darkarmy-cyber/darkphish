# Changelog fragments

Every pull request that changes runtime behavior must add one Markdown file to
this directory. Use a unique lowercase filename ending in `.md` and this form:

```text
---
category: Security
version: 0.3.0
---
- Describe one operator- or user-visible change.
```

Allowed categories are `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`,
`Security`, and `Breaking`. Release preparation validates and aggregates the
fragments into `CHANGELOG.md`, advances `VERSION` by one normal minor release
when needed, and removes consumed files. A fragment may target the current
unreleased version or exactly its next minor version; patch and skipped-minor
versions require an explicit release-policy change.
