# Darkphish 0.4 CodeQL remediation record

This is a static-analysis baseline record, not a claim that Darkphish has no
vulnerabilities. Release verification and final dispositions are pending until
the protected default-branch analyses and native publication complete.

## Authoritative starting inventory

The authenticated GitHub Code Scanning API returned **10 open alerts: 1 critical,
7 high, 2 medium** on protected `main` at
`fd3291af0e3f4b0e2edf0f2427d770be0ffb6b8d`. This matches the reported inventory.
All use CodeQL 2.26.4. Initial state is `open` for every row below.

The API does not expose a creation-commit field on an alert. We verified the
first analysis containing each alert instead: commit
`0338450b46c2115c992d0b98dfa20368457a5312`, Go analysis `1730433029`
(2026-09-05 23:13:41 UTC) and JavaScript analysis `1730431088`
(23:12:39 UTC). All predate the v0.3.0 release commit
`9e843a27e17361012e731b5c3bb9d084c223abfc`.

The latest starting instances are on `refs/heads/main`, commit `fd3291af...`,
analysis key `.github/workflows/codeql.yml:analyze`, with category suffix
`/language:go` or `/language:javascript-typescript`. Latest starting analyses:
Go `1731417282`, JavaScript `1731416884`. Locations below are **starting** locations,
not moving line numbers in the remediation branch. Start/end line are identical.

| Alert | Rule ID / name | Security severity / result level | CWE | Starting location |
| --- | --- | --- | --- | --- |
| 6 | `go/request-forgery` — Uncontrolled data used in network request | critical / error | 918 | `controllers/api/import.go:125` |
| 10 | `go/disabled-certificate-check` — Disabled TLS certificate check | high / warning | 295 | `controllers/api/import.go:121` |
| 9 | `go/reflected-xss` — Reflected cross-site scripting | high / error | 79, 116 | `middleware/middleware.go:42` |
| 7 | `go/sql-injection` — Database query built from user-controlled sources | high / error | 89 | `models/group.go:322` |
| 5 | `js/incomplete-multi-character-sanitization` — Incomplete multi-character sanitization | high / warning | 20, 80, 116 | `static/js/src/vendor/datetime-moment.js:44` |
| 4 | `js/incomplete-multi-character-sanitization` — Incomplete multi-character sanitization | high / warning | 20, 80, 116 | `static/js/src/vendor/datetime-moment.js:35` |
| 3 | `js/incomplete-sanitization` — Incomplete string escaping or encoding | high / warning | 20, 80, 116 | `static/js/src/vendor/ckeditor/plugins/wsc/dialogs/ciframe.html:14` (first replacement) |
| 2 | `js/incomplete-sanitization` — Incomplete string escaping or encoding | high / warning | 20, 80, 116 | same file/line (second replacement) |
| 1 | `js/overly-large-range` — Overly permissive regular expression range | medium / warning | 20 | `static/js/src/app/autocomplete.js:57` |
| 8 | `go/unvalidated-url-redirection` — Open URL redirect | medium / warning | 601 | `controllers/route.go:356` |

## Critical import finding: #6 (relocated as #11)

Source: authenticated request body decoded into `cloneRequest.URL`. Starting sink:
`http.Client.Get`; hardened sink: `NewRequestWithContext` / `Client.Do` in
`controllers/api/import_fetch.go`. The URL is operator-controlled and the request
is reachable in an ordinary deployment, **only on the administrative API**.
Authentication, account-state enforcement, write authorization and the
`landing-pages:write` PAT scope precede this handler. The simulation router does
not expose the import API.

The relevant guard is not a textual URL sanitizer: `dialer.Dialer()` installs
`restrictedControl` as `net.Dialer.Control`. It validates the **resolved connection
address immediately before connecting**, for each IPv4/IPv6 attempt, including
redirect destinations. It unmaps IPv4-mapped IPv6 addresses and denies configured
local, private, metadata, reserved and multicast ranges unless the administrator
explicitly allowlisted that range. This is not a resolve-then-redial check subject
to DNS rebinding. The import transport has no proxy, alternate TLS dialer or cookie
jar; it sends a GET without forwarding the inbound Authorization header, session,
captured passwords, PAT or encryption keys. Remote HTML is parsed, not executed
by the server. The custom dialer is absent from the reported CodeQL URL flow.

Concrete evidence combines that call graph/control implementation with isolated
HTTP tests: unauthenticated import rejected; direct loopback, localhost DNS and
IPv4-mapped loopback denied before the synthetic server receives a request;
explicit narrow allowlist succeeds; a redirect from an allowed destination to a
different unapproved address is denied. These boundary tests passed before the
0.4 fix. Existing dialer tests also cover metadata denial and external addresses
without making external network calls.

**Classification: FALSE POSITIVE — PROVEN for unrestricted/internal-destination
request forgery; individual GitHub disposition pending.** No connection-policy
bypass, anonymous listener crossover, code execution, filesystem access or
credential exposure was established in v0.3.0. This classification rests on the
mandatory connect-time guard, not merely on passing tests or intended behavior.
An administrator who explicitly allowlists an internal service grants access to
that service; broad allowlists remain an operator risk.

Defensive hardening in `af59c1ae548725c89d2142ba33f7fccdbf802744`: HTTP(S)-only
absolute URL validation, no userinfo, bounded URL, request deadline, header and
decompressed body limits, bounded redirects, no HTTPS downgrade, trusted TLS,
safe HTML attribute creation for import metadata and generic upstream errors.
These changes do not sanitize or disable authorized simulation HTML. Tests live
in `controllers/api/import_security_test.go` and `import_fetch_test.go`.

## Confirmed TLS defect: #10

Source: remote HTTPS certificate; sink: import transport configured with
`InsecureSkipVerify: true`. Reachable through the same authorized administrative
import path. An active network attacker or untrusted endpoint could supply import
content without proving server identity in v0.3.0. There is no implicit forwarding
of Darkphish credentials. The isolated untrusted-certificate regression failed
before the fix because the old importer accepted the response.

Fix: normal certificate/hostname validation, TLS 1.2 minimum, no insecure
downgrade. Regression proves default rejection and a legitimate HTTPS response
using a trusted synthetic test root. Users should upgrade for this import trust
fix and provision a trusted server certificate. No confirmed **critical** exploit
required a private security advisory, and no exploit recipe is published here.
Resolution: **FIXED**, commit `af59c1ae548725c89d2142ba33f7fccdbf802744`;
default-branch closure pending protected merge.

## Administrative serialization: #9

SARIF source paths are template POST/PUT request bodies, through
`Template.Validate` and `Attachment.Validate`, base64 decoding, ZIP member reads,
then the non-XML member string passed to `newZipFile.Write`. The trace attributes
that interface call to `statusRecorder.Write` and then `http.ResponseWriter.Write`.
That runtime dispatch cannot occur: `newZipFile` comes from `zip.Writer.Create`,
whose writer was constructed by `zip.NewWriter(new(bytes.Buffer))`. Neither this
buffer nor the ZIP entry writer is an HTTP writer, and neither is supplied by the
request. `Attachment.Validate` discards the resulting reader. Actual template API
responses pass through `JSONResponse` (`json.MarshalIndent` with HTML escaping,
`application/json`), with attachment content retained as base64.

Template requests and attacker-controlled member bytes are reachable after
administrative authorization, but the reported raw-HTML HTTP sink is not.
`TestTemplateAttachmentAdministrativeSerialization` exercises both authenticated
routes including the audit writer, with a synthetic non-XML ZIP member: one valid
escaped JSON response, no raw ZIP/HTML response, and unchanged simulation content
after JSON decoding. The test passes without altering the runtime writer.

**Classification: FALSE POSITIVE — PROVEN**, individual GitHub disposition pending.
Do not remove the audit wrapper, globally sanitize templates, or rewrite the ZIP
implementation to conceal this analysis limitation.

## Recipient predicate: #7

Source: group POST/PUT recipient fields; sink: `tx.Where(t).FirstOrCreate(&t)`.
Authenticated group writers can supply those fields. `Target` is a concrete
struct, not caller-supplied SQL; its ID is excluded from JSON. Inspection of
GORM v1.9.16 `Scope.buildCondition` confirms field values use `AddToVars`, with
field/column names obtained from the static model. No SQL injection was proven.
However, the struct overload omits empty fields and can reuse a different
recipient's nonempty details. The new regression reproduced this before the fix.

Fix: fixed SQL predicate with four bound parameters, including empty values.
Exact matching/reuse is preserved, owner checks unchanged. The shared
`exerciseGroupInputIsolation` fixture covers SQL-shaped synthetic strings,
empty fields, distinct/exact identity and owner isolation on SQLite, strict
MySQL and PostgreSQL. Resolution: **FIXED by explicit parameterization and identity
hardening**; commit and full hosted matrix pending.

## Frontend parsers: #2–5

For #4/#5, cell text flows into a tag-stripping regular expression and then strict
Moment date validation or numeric sort-key generation. There is **no DOM output
sink** in either function; CodeQL flags incomplete sanitization, not a demonstrated
admin-origin execution path. Administrative date cells are produced as plaintext.
Fix: remove HTML rewriting, parse the original value strictly, preserving valid
date ordering and empty-cell behavior. Markup is invalid rather than purportedly
sanitized. Both operations are covered by `scripts/frontend-security.test.mjs`.

For #2/#3, a query parameter name flows into two first-occurrence replacements
then a dynamic regular expression. The bundled bridge calls this with constant
`cmd` and `data`; the location query can be attacker-controlled if the bridge is
navigated to. Incomplete escaping is real, but those call sites do not establish
arbitrary name injection or HTML execution. Fix: literal field-name comparison;
preserve raw encoded values and first-match behavior required by XDTMaster,
including embedded equals signs. The test covers both legitimate protocol names
and regex metacharacter names without any DOM execution.

Resolution for each of #2, #3, #4 and #5: **FIXED**, commit/hosted closure pending.
The regression cases failed on the old implementations. Vendored helper changes
are deliberately narrow; there is no broad editor upgrade or dependency churn.

## Medium findings: #1 and #8

#1: text typed in the administrative template editor reaches an autocomplete
matcher. `[A-z]` includes punctuation; the matcher returns text offsets, not an
executable value. Fix `[A-Za-z]`; tests preserve every supported template name
and reject the intervening ASCII punctuation. **FIXED**, commit/hosted closure pending.

#8: unauthenticated login `next` value reaches the post-login HTTP redirect.
The old path rewriting prepended a single slash, so no external redirect was
demonstrated, but it accepted ambiguous/unknown paths and action routes. Fix:
select known administrative navigation pages and parse numeric campaign IDs;
reject absolute/authority URLs rather than rewriting them. Query/fragment removal
matches previous behavior. `/logout` and `/impersonate` are not return destinations.
Simulation campaign redirects are untouched. The regression covers legitimate
navigation, schemes, authorities, backslashes, encoded separators, traversal,
control characters and unknown/action routes. **FIXED**, commit/hosted closure pending.

## Related variants and preserved boundaries

- Other model predicates use fixed clauses/bound values. Dynamic migration/secret
  rotation identifiers come from static internal table/column lists, not request data.
- SMTP and IMAP use the restricted dialer. Their existing explicit operator
  `IgnoreCertErrors` option is not the importer's unconditional verification bypass.
  Keep certificate checks enabled in production; broader policy changes are separate.
- The application installs its restricted transport for webhooks; redirects there
  are disabled. Vault uses an operator-configured HTTPS service/CA and bounded
  client, not the untrusted site-import URL. No shared egress guard was weakened.
- Template API return paths use JSON serialization. No template HTML is newly
  rendered on the administrative origin. Raw simulation HTML and Office attachment
  template support remain deliberately intact.
- No changes to credential modes, encrypted storage, policy evaluation, reviewer
  scoping, reauthentication, individual audited reveal, retention or no-store UI.
  No replay, external password checks, bulk plaintext export or evasion was added.

## Hosted cycles and release gate

| Cycle | Source | Hosted result | CodeQL observation |
| --- | --- | --- | --- |
| 1 | `af59c1a` (PR merge analysis `08dcf87e`) | CI `34026098007`, CodeQL `34026097988`: all ten required checks pass, including Linux race and both database jobs | PR incremental analysis reported relocated #11 (Go 1, JS 0); this does not measure inherited main alerts. No dismissal yet. |

Codex review of `af59c1a` reported no major issues. Security-mode review was also
requested; final review coverage must be verified on the completed diff.

Local first-batch validation passed unit tests, vet, staticcheck, build, frozen
frontend install/build/audit, actionlint, changelog and automation tests. Windows
race failed in ThreadSanitizer allocation before execution; hosted Linux race
passed. Govulncheck found no reachable/imported-package vulnerabilities; its
unused OpenPGP module advisory is not suppressed.

After remediation, a read-only fail-closed default-branch alert diagnostic will
be integrated with preparation and publication. The baseline, protected merge,
release tag, seven native assets and final source scan remain to be verified.
