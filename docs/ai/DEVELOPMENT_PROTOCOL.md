# AI Development Protocol

Purpose: keep agent-assisted Darkphish development precise, auditable and token-efficient without reducing engineering or security quality.

## Context hierarchy

For each task, consume context in this order:
1. Current task/release specification.
2. Canonical architecture/security/database/testing documentation relevant to the task.
3. Symbol/file search for the affected implementation.
4. Only then expand repository exploration when evidence shows it is necessary.

Do not repeatedly rediscover settled architecture.

## Task protocol

1. Identify the DP task ID and acceptance criteria.
2. Search for affected symbols and tests.
3. Read the minimum coherent implementation slice.
4. State any new assumption that is not already specified.
5. Implement the smallest maintainable change satisfying the task.
6. Run targeted tests for affected packages/modules.
7. Update/add tests and durable documentation where behavior changed.
8. Commit a cohesive change.

## Integration protocol

After a coherent batch:
- run broader affected-module tests;
- inspect migrations/API compatibility where relevant;
- perform one normal review;
- fix substantiated findings.

At release candidate:
- run the full protected test/database/security suite;
- perform one final security review;
- resolve release blockers;
- verify artifacts after publication.

Do not run expensive full-repository analysis after every trivial edit unless the change is cross-cutting or policy requires it.

## Durable memory

Record stable information in the repository rather than restating it in prompts. Prefer:
- release specifications in `docs/releases/`;
- architectural decisions in `docs/adr/`;
- canonical architecture/security/testing documents;
- a concise current-state document when needed.

## Prompt shape

Preferred task prompt:

`Implement DP-08X from docs/releases/0.8.md. Follow canonical repository/security rules. Search/read only the code needed for this task. Run targeted tests. Do not broaden scope. Report changed files, tests, assumptions and remaining risks.`

This keeps prompts short while preserving authoritative context in version control.

## Review discipline

Default:
Implementation -> targeted tests -> normal review -> fix -> final security review at release gate.

Additional review loops require a concrete failure/finding, not habit.

## Non-negotiable safety

Token efficiency never justifies skipping authorization checks, credential-safety invariants, migrations, required release gates, relevant tests or security review.
