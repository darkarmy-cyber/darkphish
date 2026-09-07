# Darkphish 0.7 human security foundations

Darkphish 0.7 starts from the immutable `v0.6.0` release at
`c8ac718249d3f9a5796a2345759bd8879c99d4d1`. The release establishes three
foundations that will evolve together: a provider-neutral employee directory,
typed Smart Groups, and an explainable Human Risk Engine.

## Product and security boundary

Darkphish remains an authorized security-awareness and phishing-simulation
platform. Administrator accounts and employees are separate identities: an
employee imported for awareness activity is not an application login. Existing
static groups and historical campaign recipients remain supported.

The 0.7 risk model is deterministic and explainable. It may use only operational
security evidence already represented by Darkphish, such as simulated link
clicks, submitted-data events, reported simulation messages, irreversible local
credential-policy findings, new-joiner state, and operator-managed privileged or
critical-role flags. It must not use protected personal traits, arbitrary
free-text metadata, or make employment/HR decisions.

Raw submitted credentials are never copied into employee, Smart Group, risk,
audit, log, or reporting records. The existing `disabled`, `policy_only`, and
`encrypted_review` credential modes, privileged reauthentication, audited
individual reveal, retention, SecretStore handling, and local password-policy
evaluation remain unchanged. No credential replay, password spraying, credential
stuffing, external password validation, mail-filter bypass, anti-sandbox, or
defensive-evasion capability belongs in this workstream.

## Directory domain

Directory data is owner-scoped by Darkphish `user_id`. A `directory_source`
represents a provider configuration; an `employee` represents a managed campaign
subject. Stable provider IDs are preferred for reconciliation, while normalized
email is a second owner-scoped identity guard. Mutable email, department, title,
manager, and location attributes do not create application accounts.

The first production-capable synchronization path in 0.7 will be a deterministic
manual/CSV provider with bounded input, preview, collision detection, and audited
reconciliation. The provider boundary must later accept Google Workspace,
Microsoft Entra ID, Okta, and SCIM without rewriting employee, Smart Group, or
risk logic. Provider secrets, when connectors are introduced, must use the
existing SecretStore architecture and secret-free diagnostics.

Synchronization semantics are fail-closed and idempotent: create new employees,
update attributes while preserving stable identity, mark departed/suspended
employees without deleting campaign history, reactivate only when the source
explicitly does so, and reject ambiguous identity collisions. Directory sync
must never launch a campaign as a side effect.

## Smart Groups

Static groups remain the default legacy behavior. A Smart Group is represented
by a separate definition attached to a normal group ID, so existing group rows
and campaign references do not need to be rewritten. Its rule document is a
versioned, bounded, typed AST: no raw SQL, regex execution, templates, or
scripting language.

Initial rule inputs are limited to directory/employee attributes and derived
security facts: department, job title, directory source, employment status,
joined age, privileged/critical-role flags, current risk score/category, recent
simulation failures, recent submitted-data events, and recent reporting behavior.
Rule trees have explicit depth/rule/value limits and deterministic `all`/`any`
composition. Evaluation returns match explanations. Human Risk never depends on
Smart Group membership, so Smart Groups may consume risk without creating a
recursive scoring loop.

## Human Risk Engine v1

Policy `human-risk-v1` produces a score from 0 to 100 with categories Low,
Medium, High, and Critical. Every contribution has a stable factor key,
contribution, source count/window, policy version, and computation timestamp.
The same normalized evidence at the same evaluation time produces the same
score. Positive reporting behavior can reduce risk; simulated failures and
locally-derived weak-policy findings can increase it. Privileged/critical-role
and new-joiner flags are transparent exposure factors, not hidden ML features.

Current scores and bounded historical snapshots are stored separately. A
DB-backed recompute queue will allow event-triggered or explicit refresh without
Redis, Kafka, or another coordination dependency. Multi-instance processing must
remain idempotent and compatible with the 0.6 database-coordinated audit model.

## Audit, authorization, and privacy

Directory source mutations, sync summaries, privileged/critical-role changes,
Smart Group mutations, risk-policy changes, and explicit recomputes are
security-relevant administrative actions and must use the durable audit outbox.
New API surfaces will receive least-privilege RBAC/PAT scopes rather than relying
on hidden UI controls. Security Reviewer does not automatically receive directory
or organization-wide risk administration privileges.

Human Risk is for security prioritization and coaching, not automatic discipline.
Deployments should minimize imported directory attributes, define risk-history
retention, and restrict individual risk explanations to authorized operators.

## Compatibility and release gates

Goose remains the only schema authority for SQLite, MySQL, and PostgreSQL. 0.7 is
additive over 0.6 and must preserve audit events/checkpoints/signing identities,
credential ciphertext, campaigns, static groups, users, tokens, and reviewer
state. SQLite remains supported; MySQL/PostgreSQL remain the supported
multi-instance network databases.

The engineering branch keeps root `VERSION` at the released baseline while 0.7
changelog fragments target `0.7.0`; trusted release preparation performs the
version advance. All ten protected checks, hosted race testing, both CodeQL
languages, zero open CodeQL alerts, normal/security review, protected merge,
native assets, checksums, SPDX SBOM, and released-binary smoke remain release
gates. Docker is not a product or release gate.

Do not begin Darkphish 0.8 until `v0.7.0` is actually published and verified.
