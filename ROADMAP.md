# Darkphish Product Roadmap

Darkphish evolves from a phishing simulation framework into an open-core Human Risk Management platform. One minor release should introduce one primary product capability; security, compatibility, testing, and migration gates remain mandatory for every release.

## Product loop

Directory -> Smart Groups -> Simulation -> Behaviour Evidence -> Human Risk -> Training -> Retest -> Risk Trend

## Releases

### v0.8 - Campaign Engine 2.0
Recurring and scheduled campaigns, timezone-aware delivery, bounded randomized delivery windows, Smart Group targeting, exclusions, cloning, pause/resume, lifecycle state machine, preview/test-send, template versioning, and a safe synthetic test mode.

### v0.9 - Training Engine
Micro-training, training assignments, completion evidence, quizzes, localization, and outcome-driven reinforcement.

### v0.10 - Human Risk
Complete explainable user/group/organization risk scoring, bounded history, policy versioning, trends, and transparent evidence attribution.

### v0.11 - Adaptive Engine
Policy-controlled simulation cadence and difficulty based on explainable risk and prior security-awareness evidence. No covert evasion or mail-filter bypass capabilities.

### v0.12 - Directory & Identity
Microsoft Entra ID and Google Workspace first, followed by standards-based/controlled import mechanisms such as SCIM and CSV. User lifecycle and organizational attributes feed Smart Groups.

### v0.13 - Reporting
CISO dashboard, organizational and departmental trends, campaign comparisons, training compliance, repeat-risk indicators, management exports, and audit evidence. Include a board-oriented executive report.

### v0.14 - Integrations
Stable API/webhooks plus defensive integrations for SIEM/SOC and collaboration/reporting workflows. Prioritize Microsoft 365, Google Workspace, Teams/Slack/Google Chat and common SIEM platforms.

### v0.15 - AI Assistance
Human-reviewed generation of simulation/training drafts, policy-derived training drafts, campaign recommendations and analytical summaries. AI output must remain explainable, reviewable and bounded by product safety controls.

### v0.16 - Automation
Policy engine connecting risk, Smart Groups, simulations, training, retesting and updated risk. Administrators define guardrails; automation remains auditable and reversible.

### v0.17 - Multi-tenancy / MSP
Strong tenant isolation, delegated administration, centralized MSP console, tenant reporting and optional white-label capabilities.

### v0.18 - Enterprise
SSO/OIDC/SAML, granular RBAC, enterprise audit/retention controls, delegated administration, hardened API credentials and enterprise policy controls.

### v0.19 - Production Hardening
No major feature expansion. Browser E2E, supported-database matrix, upgrade/migration tests, performance and scale testing, backup/restore validation, HA readiness, installer/upgrade UX and documentation.

### v1.0 - Commercial GA
Stable contracts, documented upgrade path, supported deployment model, licensing/entitlements, operational documentation, support policy and production readiness evidence.

## Permanent safety boundaries

Darkphish is for authorized security-awareness simulation and defensive human-risk management. Raw submitted credentials must never enter risk storage, logs, normal APIs, webhooks, directory providers or external validation. Do not add credential replay/stuffing/spraying, external captured-password validation, mail-filter bypass, anti-sandbox, anti-detection or defensive-evasion functionality. Preserve the existing credential safety modes and explicit auditability.

## Release discipline

- One primary capability per minor release.
- Release specification exists before implementation begins.
- Database migrations remain backward-conscious and tested on every supported database.
- Targeted tests run during implementation; the complete protected suite runs before merge/release.
- Normal code review and final security review are required before release.
- Do not begin the next release until the current release is genuinely published and verified.
