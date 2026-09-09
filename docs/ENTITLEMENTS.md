# Entitlement Architecture

## Goal

Provide one centralized, testable capability/limit decision layer for Community, Professional, Enterprise and MSP packaging without scattering commercial logic throughout the application.

## Conceptual interface

Callers ask the entitlement service for a capability or limit, for example:

- `campaign.active.max`
- `campaign.recurring`
- `campaign.advanced_scheduling`
- `training`
- `smart_groups`
- `risk_engine`
- `directory.entra`
- `directory.google`
- `automation`
- `ai.assistance`
- `sso`
- `rbac.advanced`
- `audit.advanced`
- `integrations.siem`
- `multi_tenant`
- `white_label`
- `users.managed.max`

Avoid edition-name checks such as `if enterprise` in feature code.

## Required properties

1. Deterministic: identical license state yields identical entitlements.
2. Centralized: entitlement parsing/evaluation has a single authoritative implementation.
3. Auditable: license installation/change and meaningful entitlement-state transitions are audit events.
4. Offline-capable: on-premise commercial deployments can validate signed licenses without continuous vendor connectivity.
5. Fail-safe: malformed/expired commercial licenses never damage or delete data.
6. Export-safe: expiration does not prevent customers from exporting their own data.
7. Security invariant: authentication, authorization, encryption, audit integrity and credential-safety controls are never weakened by edition.
8. Testable: edition matrices and boundary values have unit/integration coverage.

## Proposed model

A signed license document identifies a deployment/customer, validity window and explicit entitlement set/limits. Verification uses an embedded public verification key; private signing material never ships with Darkphish.

Runtime flow:

License source -> Parser -> Signature/validity verification -> Normalized entitlement set -> Entitlement service -> Feature boundary

Community operation uses a built-in deterministic Community entitlement set and requires no license file.

## Initial Community limits

- `campaign.active.max = 1`
- `users.managed.max = 100`
- advanced commercial capabilities default false

These values are product assumptions until commercial validation is complete; implementation should keep limits configurable in the entitlement definition rather than hard-coded throughout feature code.

## Repository separation

The public core should own stable entitlement interfaces and Community behavior. Proprietary implementations/modules may live in a private commercial repository or private build layer. The public core must remain buildable and useful independently.

## Anti-patterns

- Network call to a vendor server on every request
- Obfuscated secrets embedded in the binary as the primary protection
- Disabling security controls when a license expires
- Deleting or encrypting customer data after expiry
- Hundreds of UI/backend `edition == ...` conditionals
- Licensing checks embedded in database migrations
