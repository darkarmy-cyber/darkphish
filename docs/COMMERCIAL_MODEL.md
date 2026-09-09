# Darkphish Commercial Model

## Strategy

Darkphish should use an open-core model: a genuinely useful Community edition remains open source, while advanced operational, enterprise and MSP capabilities are commercially licensed. Avoid intentionally unusable crippleware; Community should be capable of a real authorized small-scale awareness program.

## Proposed editions

### Community - free/open source
- One active campaign at a time
- Up to 100 managed users
- Core users/groups/templates/landing pages/sending profiles
- Basic simulation workflow and reporting
- Core supported databases
- Basic API required for normal operation

### Professional - commercial
- Unlimited active campaigns subject to deployment capacity
- Campaign Engine 2.0 advanced scheduling/recurrence
- Smart Groups
- Human Risk capabilities
- Training Engine
- Advanced analytics/reporting
- Directory integrations
- Automation
- Approved AI-assisted workflows

Indicative packaging target: EUR 1.50-3/user/month with a minimum monthly subscription. Final pricing requires customer validation.

### Enterprise - commercial
Professional plus:
- SSO/SAML/OIDC
- Advanced RBAC and audit
- Enterprise retention/policy controls
- SIEM and enterprise integrations
- Hardened API/integration controls
- Enterprise support options

Indicative packaging target: EUR 3-6/user/month or contract pricing. Final pricing requires validation.

### MSP - commercial
- Strong multi-tenancy
- Centralized administration
- Delegated tenant administrators
- Cross-tenant operational overview without cross-tenant data leakage
- Tenant-specific reporting
- Optional white-label controls
- MSP-oriented API and commercial terms

## Architecture principle

Commercial capability must not be implemented through scattered ad-hoc `if paid` checks. All edition decisions flow through a centralized Entitlement Engine documented in `docs/ENTITLEMENTS.md`.

Prefer separation between the public core and private commercial modules where this reduces trivial bypass, protects proprietary enterprise functionality, and preserves clean interfaces. Public interfaces must not contain secrets or require online license checks for core Community operation.

## Licensing principles

- Community remains useful without network access to a licensing service.
- Commercial licenses should support controlled offline/on-premise environments.
- License verification failure must fail safely and predictably; it must never corrupt campaign or audit data.
- Entitlements are explicit capabilities/limits, not product-name conditionals.
- Security controls are never disabled because of a lower commercial tier.
- Existing customer data remains exportable when a commercial license expires.
- License state changes are auditable.

## Commercial validation before final pricing

Interview at least 10 SMB/mid-market security buyers and 5 MSP/MSSP operators. Validate willingness to pay, preferred metric (managed users vs active users vs tenant), procurement thresholds, on-premise requirements, SSO expectations, directory requirements, reporting requirements and support expectations before locking GA pricing.
