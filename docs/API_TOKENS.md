# Personal access tokens

Create tokens under Account Settings or `POST /api/pats/` with a name, explicit
scope list, and RFC 3339 expiry. The response shows a value in this form once:

```text
darkphish_pat_<prefix>_<secret>
```

Darkphish stores the prefix and a SHA-256 digest of the 256-bit random secret,
never the raw value. Authentication performs a constant-time digest comparison,
checks expiry and revocation, applies token scopes, then applies the user's
current account state and role permissions. Last-use updates are throttled.

Available scopes are `audit:read`, `campaigns:read`, `campaigns:write`,
`credentials:view`, `groups:read`, `groups:write`, `integrations:read`,
`integrations:write`, `landing-pages:read`, `landing-pages:write`,
`reports:read`, `sending-profiles:read`, `sending-profiles:write`,
`templates:read`, `templates:write`, `tokens:manage`, `users:read`, and
`users:write`.

List and revoke your own tokens with `GET /api/pats/` and
`DELETE /api/pats/{id}`. Creation returns the raw value only after the digest and
durable audit-outbox event commit together; revocation uses the same transaction
model. A failed write never returns a usable raw token.

A PAT with `credentials:view` does not receive a browser privileged session and
does not bypass authorization. Its current user must also be active, hold the role
permission, and be a System Administrator or have an unexpired Security Reviewer
assignment for the campaign. Legacy permanent API keys and the `/api/reset`
rotation behavior remain disabled.
