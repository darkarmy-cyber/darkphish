# Warm graphite administrative interface

This is the first implementation slice of the approved DarkPhish enterprise redesign: a shared visual foundation across the existing product, real sign-in/password reset and spacious editors. It is not a new frontend framework or an authentication/authorization change.

## Design contract

- Administrative pages opt in with `body.enterprise-ui`; authentication uses `body.auth-page`.
- `static/css/enterprise.css` is compiled **last** by the existing Gulp styles task into the shipped `static/css/dist/darkphish.css`. Rebuild the bundle whenever the source changes.
- Warm canvas `#f5f3ee`, paper `#fffefa`, ink `#26352f`, muted text `#606a63`, primary action `#245d4e`. Normal text/primary/status pairings have automated contrast checks. Colour is not the only indicator of state.
- Fonts use the local system stack; icons and the unchanged transparent DarkPhish fish/favicon are served locally. No network font dependency, replacement logo or tracking.
- Sidebar navigation remains available on small screens. Account, notifications and sign-out are never hidden by a collapse breakpoint. Long usernames remain truncated within the account control.
- Keyboard focus remains visible. Reduced-motion preferences are honoured. Native labels and password-manager autocomplete remain intact.
- Semantic metric colours are retained for chart marks; numeric table values use readable ink. No metrics, event classifications or denominators are changed.

## Authentication and security

The sign-in and reset forms keep their existing POST targets, field names, autocomplete attributes, required inputs, minimum password length, flash rendering and password-strength integration. There is no new SSO/MFA implementation and no alteration to server-side authentication, request validation, permissions, update verification or licensing.

Template content rendered by CKEditor and public landing pages do not inherit the administrative theme. Existing sanitization, image-preview consent, external-address checks and source preservation remain unchanged. No remote images are automatically loaded by this redesign.

## Editor compatibility

Email and landing-page editors now use a wide workspace surface. They intentionally retain their existing Bootstrap dialog lifecycle, IDs, import dialogs, save handlers and focus integration with CKEditor. This avoids changing persistence while the visual system is introduced. These are wide workspaces, **not new route-addressable editor pages**.

## Remaining design stages

The full proposal is broader than this foundation. Dedicated editor routes with unsaved-change protection, a multi-step campaign setup/review flow, a restructured analytics dashboard, and a separately designed dark theme remain follow-up work. Do not represent this PR as completion of those stages. Existing campaign confirmations and license gates remain in force; this work neither introduces a draft persistence model nor bypasses sending restrictions.

## Verification

- `node --test scripts/enterprise-ui.test.mjs scripts/branding.test.mjs` checks bundle wiring, scopes, authentication contracts, original brand assets, editor integration and text contrast.
- Run the existing complete Node test suite and controller tests. Some existing repository source-text tests assume LF checkouts; Windows CRLF failures are not a reason to weaken the assertions.
- Browser checks must render the real Go templates with the shipped bundle, not only the design prototype. Cover 320, 375, 768, 1024, 1440 and 1920 px; empty/populated tables; login errors; role-gated settings; CKEditor and nested imports.
- No real license activation, SMTP traffic, update apply, production migration or server deployment is needed for UI verification.
