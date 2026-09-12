# Changelog fragments

Every pull request that changes runtime behavior must add one Markdown file to
this directory. Use a unique lowercase filename ending in `.md` and this form:

```text
---
category: Security
version: 0.7.1
---
- Describe one operator- or user-visible change.
```

Allowed categories are `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`,
`Security`, `Breaking`, and `Migration`. Release preparation validates and aggregates the
fragments into `CHANGELOG.md`, advances `VERSION` to the selected release target,
and removes consumed files.

A fragment may target exactly one supported release target for the repository
state being prepared:

- the current unreleased version;
- exactly the next normal minor version; or
- the next patch version of the current released minor when the change is an
  exceptional production hotfix permitted by `docs/RELEASES.md`.

Do not mix patch and minor targets in the same repository state. Patch hotfixes
must remain narrowly scoped to regression, security, data-integrity, or
production-blocking fixes and must not carry unrelated next-minor features.
Skipped-minor and other version jumps require an explicit release-policy change.
