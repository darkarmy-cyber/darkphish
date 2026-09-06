# Darkphish 0.5 persistence modernization

## Baseline and scope

Inventory captured **before replacing GORM v1**, at protected v0.4.0/main
`ae36e9c10ef23c1b20438bd219a3d51c9d9b47cc`, on 2026-09-06.
The live protected-main CodeQL inventory is zero. The pre-existing untracked
`darkphish-master.zip` and maintenance PRs #1–5 are outside this workstream.

Runtime baseline: `github.com/jinzhu/gorm v1.9.16`,
`github.com/lib/pq v1.12.3`, MySQL driver `v1.10.1`, SQLite driver
`v1.14.32`, Goose `v3.27.3`, Go `1.27.0` / toolchain `go1.27.1`.

There are **25 legacy ORM import statements in 24 runtime files** (including
the PostgreSQL dialect registration), plus test imports. The receiver-root
review index below contains **519 candidate fluent ORM calls**, including
connection, query, transaction and metadata operations; chained methods are
separate calls, not separate queries. ZIP/DOM/secret-provider calls with the
same method names are excluded. This lexical review index is a navigation aid,
not a type-checked vulnerability or query count. The runtime imports are an
unambiguous exit metric. All nine model hooks are separately listed.

## Maintained target versions

Resolved from the official Go module proxy on 2026-09-06, before changing
dependencies:

| Component | Stable target | Published |
| --- | --- | --- |
| GORM v2 API, `gorm.io/gorm` | v1.31.2 | 2026-06-22 |
| `gorm.io/driver/sqlite` | v1.6.0 | 2025-06-04 |
| `gorm.io/driver/mysql` | v1.6.0 | 2025-06-04 |
| `gorm.io/driver/postgres` | v1.6.2 | 2026-07-31 |

The v2 API deliberately uses a v1.x module tag. Its PostgreSQL driver is pgx
backed. Final transitive versions and changes will be recorded after tidy.
Do not upgrade Goose, the existing native SQLite/MySQL drivers or frontend/
workflow dependencies merely because maintenance PRs exist.

Primary migration references:
[GORM v2 changes](https://gorm.io/docs/v2_release_note.html),
[driver/connection documentation](https://gorm.io/docs/connecting_to_the_database.html),
[transactions](https://gorm.io/docs/transactions.html),
[updates](https://gorm.io/docs/update.html),
[associations](https://gorm.io/docs/associations.html).

## Domain and semantic inventory

| Domain / files | Existing patterns and boundary | Required v2 treatment |
| --- | --- | --- |
| Connection, migration inspection: models.go, migration_check.go | Two gorm.Open sites; underlying sql.DB pools/Ping/Close; Goose Up; metadata-only HasTable and SQLite query_only | One explicit dialect factory; safe pool ownership/error redaction; Goose remains sole schema authority; no AutoMigrate |
| Users/authentication: user.go, rbac.go, nullable_time.go | First+Preload Role; Save all values; protected account mutation + grant revocation + outbox transaction; role-permission association lookup | Preserve false/empty/NULL updates; explicit errors.Is; no automatic writes to supplied Role; tested permission predicates |
| PATs: pat.go, security_repositories.go | Create/revoke plus audit outbox in withSecurityTransaction; bound owner/status clauses; RowsAffected distinguishes absent/revoked | Propagate original errors, commit failures; no raw token on failed commit; preserve conditional revoke |
| Privileged grants/reviewers: privileged.go, reviewer.go | Session hash lookup, nullable expiration, campaign-owner and permission checks; reviewer + outbox transaction | No zero-value omission, scoped joins, deterministic ordering and revocation/expiry parity |
| Credentials/policy: credential.go, security_repositories.go | Explicit replace/delete/save of finding+ciphertext in one transaction; audited reveal/purge/rotation retain their existing boundaries | Preserve exact ciphertext and retention; roll back finding replacement if ciphertext write fails; audited mutations roll back with outbox failure; no new reveal authority |
| Audit/outbox/signatures: audit.go, audit_outbox.go, audit_export.go | Explicit Begin/read-last/Create/reload/hash/update/Commit; microsecond persisted values; ordered sequence/checkpoint reads; idempotent delivery ID | Keep hash of stored representation and signed checkpoints; isolate reused filters; no global/default logger payloads |
| Integration secrets: secrets_rotation.go, smtp.go, imap.go, webhook.go | Explicit seal/write/restore-memory; allowlisted table/column identifiers; per-record rotation transaction with audit | No association write may persist plaintext; no dual write/provider change; source identifiers stay static |
| Campaigns/results/events/mail queue: campaign.go, result.go, maillog.go | Three Related calls; scalar Find; explicit campaign creation transaction; reused stats query; all-row processing reset | Replace Related with explicit owner/foreign-key queries; scalar Take/First; isolated base queries; explicit bounded global-reset intent |
| Groups/targets: group.go | Explicit four-field recipient identity, manual join table, Begin/Save/Commit; map updates | Preserve empty identity values and owner isolation; ignore DTO target association; deterministic commit/error handling |
| Pages/templates/SMTP: page.go, template.go, smtp.go, modified_time.go | Scoped scalar Find, Save, explicit attachment/header lifecycle, nine legacy hook methods overall | No missing-row success; v2 hook signatures; prevent automatic association writes and plaintext leaks; preserve explicit relationships |
| Email requests: email_request.go | Save then bound first lookup | Preserve IDs, defaults and error distinction |
| HTTP adapters: controllers/api/{campaign,group,page,smtp,template,user,util}.go | ORM sentinel comparisons, no SQL construction | Incremental error boundary; use errors.Is, never translate arbitrary database errors into not-found |

### Behavioral differences requiring deliberate handling

- v1 scalar Find signals missing records; v2 Find does not. Review every scalar
  retrieval separately from slice/summary projections.
- v2 method chains share statement state. Reused statistics/filter queries must
  branch from an initialized session rather than accumulate previous clauses.
- Count requires int64; check every count, not just compilation failures.
- v1 no-argument hooks are not the v2 hook interface. Preserve normalization on
  Save/Find; map updates explicitly include false, zero, empty and NULL values.
- Legacy `sql` / snake-case association settings do not protect v2. DTO-only
  fields need explicit ignore tags; supplied role/template/page/SMTP records
  must not be silently created or updated through associations.
- v2 guards global writes. Retain that default; give intentional queue recovery
  and test cleanup explicit predicates, never enable AllowGlobalUpdate globally.
- Save can upsert and writes zero values; retain only where the domain expects
  create-or-save, and keep authentication/ownership checks before it.
- Existing explicit transactions return commit errors. Security transactions
  must also roll back on panic/cancellation; nested savepoints must not allow a
  partial business mutation to escape a failed audit operation.
- No production AutoMigrate/NewScope/custom callback registrations were found.
  Existing schema authority is Goose; test-only DDL is not production migration.

## Contract tests before migration

`databasetest/persistence_contract_test.go` extends the same SQLite, strict
MySQL and PostgreSQL security-model fixture, without three divergent suites.
It covers successful/scalar-missing/empty-list/database-error cases, generated
and explicit IDs, unique conflicts, binary values, microsecond dates, nil/NULL,
false/zero/empty transitions using Save and map updates, affected rows, explicit
pagination, independent derived queries, commit/rollback/finished transactions,
owner isolation, actual nullable/modification hooks and non-updating Role
associations. The temporary contract table is explicit test DDL and is dropped.

The existing shared security fixture continues to exercise bootstrap/login,
PAT lifecycle, cross-owner recipient identity, reviewer assignment, privileged
reauthentication (including legacy bcrypt upgrade and expiry), encrypted reveal,
durable outbox delivery, stored audit-chain/checkpoint verification and injected
outbox rollback. The 0.5 phase will expand failed security mutations and add a
real v0.4-created data fixture plus reopen/restart checks before readiness.

## Schema / compatibility policy

**No schema change is planned solely for this ORM migration.** Goose migration
files must remain unchanged. Existing 0.4 databases must be opened directly,
without export/import or automatic schema generation. Schema equality means no
schema downgrade is required to roll back to v0.4; operational backup and any
newly written application data still require ordinary rollback planning.

No TOTP/WebAuthn, multi-instance audit redesign, signing KMS, product containers,
credential replay or external password validation is included.

## Initial hook index

- `models/modified_time.go:13` — `func (g *Group) BeforeSave() error`
- `models/modified_time.go:18` — `func (t *Template) BeforeSave() error`
- `models/modified_time.go:23` — `func (p *Page) BeforeSave() error`
- `models/modified_time.go:28` — `func (s *SMTP) BeforeSave() error`
- `models/nullable_time.go:13` — `func (u *User) BeforeSave() error`
- `models/nullable_time.go:20` — `func (u *User) AfterFind() error return u.BeforeSave() }`
- `models/nullable_time.go:22` — `func (im *IMAP) BeforeSave() error`
- `models/nullable_time.go:27` — `func (im *IMAP) AfterFind() error`
- `models/nullable_time.go:34` — `func (c *Campaign) AfterFind() error`

## Initial runtime import index

- `controllers/api/campaign.go:15`
- `controllers/api/group.go:13`
- `controllers/api/page.go:13`
- `controllers/api/smtp.go:13`
- `controllers/api/template.go:13`
- `controllers/api/user.go:15`
- `controllers/api/util.go:11`
- `models/audit.go:11`
- `models/audit_export.go:10`
- `models/audit_outbox.go:10`
- `models/campaign.go:11`
- `models/credential.go:13`
- `models/group.go:10`
- `models/migration_check.go:8`
- `models/models.go:19`
- `models/models.go:20`
- `models/pat.go:17`
- `models/privileged.go:14`
- `models/result.go:11`
- `models/reviewer.go:10`
- `models/secrets_rotation.go:10`
- `models/security_repositories.go:7`
- `models/smtp.go:18`
- `models/template.go:9`
- `models/user.go:10`

## Initial receiver-root call index

All locations refer to the protected baseline above; numbers do not move with
the migration. A suffix names the individual method at that original line.

- `models/audit.go`: `114:Begin`, `120:Where`, `120:First`, `121:Rollback`, `129:Rollback`, `136:Where`, `136:Order`, `136:First`, `141:Rollback`, `148:Create`, `149:Rollback`, `155:Where`, `155:First`, `156:Rollback`, `162:Rollback`, `165:Model`, `165:Where`, `166:Updates`, `167:Rollback`, `170:Commit`, `183:Where`, `186:Where`, `189:Where`, `192:Where`, `195:Where`, `198:Where`, `201:Where`, `204:Where`, `210:Model`, `212:Count`, `222:Order`, `222:Limit`, `222:Offset`, `222:Find`, `236:Where`, `236:Order`, `236:Find`, `252:Where`, `252:Delete`, `283:Order`, `283:Find`, `306:Model`, `306:Where`, `306:Updates`, `323:Where`, `323:First`, `329:Where`, `329:Order`, `329:First`, `332:Where`, `332:First`, `344:Create`, `355:Where`, `355:Order`, `355:First`, `374:Where`, `374:Order`, `374:Find`, `388:Where`, `388:First`, `415:Where`, `415:Order`, `415:Find`, `424:Where`, `424:First`
- `models/audit_export.go`: `15:Where`, `15:Order`, `15:Find`, `101:Where`, `101:First`
- `models/audit_outbox.go`: `42:Create`, `50:Model`, `50:Where`, `50:Update`, `58:Where`, `58:Order`, `58:Limit`, `58:Find`, `68:Model`, `68:Where`, `68:Updates`, `76:Model`, `76:Where`, `76:Updates`, `80:Model`, `80:Where`, `81:Updates`
- `models/campaign.go`: `180:Table`, `180:Where`, `180:Update`, `202:Save`, `216:Model`, `216:Related`, `221:Model`, `221:Related`, `226:Table`, `226:Where`, `226:Find`, `234:Where`, `234:Find`, `239:Table`, `239:Where`, `239:Find`, `247:Table`, `247:Where`, `247:Find`, `261:Where`, `261:Find`, `306:Table`, `306:Where`, `307:Count`, `311:Where`, `311:Count`, `315:Where`, `315:Count`, `319:Where`, `319:Count`, `325:Where`, `325:Count`, `331:Where`, `331:Count`, `337:Where`, `337:Count`, `344:Model`, `344:Related`, `363:Table`, `363:Where`, `364:Select`, `365:Scan`, `389:Table`, `389:Select`, `393:Joins`, `394:Where`, `396:Where`, `398:Order`, `398:Scan`, `417:Table`, `417:Where`, `418:Select`, `419:Scan`, `440:Table`, `440:Where`, `441:Select`, `441:Scan`, `458:Where`, `458:Where`, `458:Find`, `462:Table`, `462:Where`, `462:Find`, `469:Where`, `469:Find`, `473:Table`, `473:Where`, `473:Find`, `477:Where`, `477:Find`, `487:Where`, `487:Where`, `487:Find`, `504:Where`, `504:Find`, `522:Table`, `522:Where`, `522:Find`, `530:Table`, `530:Where`, `530:Find`, `538:Table`, `538:Where`, `538:Order`, `538:Find`, `549:Where`, `550:Where`, `550:Find`, `644:Begin`, `649:Rollback`, `652:Save`, `661:Save`, `701:Save`, `720:Save`, `730:Commit`, `742:Where`, `742:Delete`, `747:Where`, `747:Delete`, `752:Where`, `752:Delete`, `758:Delete`, `776:Where`, `776:Delete`, `788:Model`, `788:Where`, `789:Select`, `789:UpdateColumns`
- `models/credential.go`: `128:Where`, `128:First`, `145:Save`, `294:Where`, `294:Find`, `304:Where`, `304:Find`, `329:Table`, `330:Joins`, `331:Where`, `332:Select`, `332:First`, `337:Where`, `337:First`, `344:Where`, `344:First`, `362:Model`, `362:Where`, `363:Updates`, `369:Model`, `370:Where`, `371:Updates`, `376:Where`, `376:Delete`, `379:Where`, `379:Delete`, `382:Where`, `382:Delete`
- `models/email_request.go`: `89:Save`, `96:Table`, `96:Where`, `96:First`
- `models/group.go`: `111:Where`, `111:Find`, `129:Table`, `129:Where`, `130:Select`, `130:Scan`, `136:Table`, `136:Where`, `137:Count`, `149:Where`, `149:Find`, `164:Table`, `164:Where`, `165:Select`, `165:Scan`, `170:Table`, `170:Where`, `171:Count`, `181:Where`, `181:Find`, `199:Begin`, `200:Save`, `202:Rollback`, `209:Rollback`, `214:Commit`, `217:Rollback`, `247:Begin`, `255:Where`, `255:Delete`, `257:Rollback`, `272:Rollback`, `281:Rollback`, `285:Save`, `290:Commit`, `292:Rollback`, `301:Where`, `301:Delete`, `307:Delete`, `324:Where`, `325:FirstOrCreate`, `332:Save`, `353:Model`, `353:Where`, `353:Updates`, `365:Table`, `365:Select`, `365:Joins`, `365:Where`, `365:Scan`
- `models/imap.go`: `130:Where`, `130:Find`, `130:Count`, `169:Save`, `180:Where`, `180:Delete`, `188:Model`, `188:Where`, `188:Update`
- `models/maillog.go`: `56:Save`, `77:Save`, `92:Save`, `98:Save`, `115:Delete`, `130:Delete`, `269:Where`, `270:Find`, `280:Where`, `280:Find`, `286:Begin`, `289:Save`, `291:Rollback`, `295:Commit`, `303:Model`, `303:Update`
- `models/migration_check.go`: `13:Model`, `13:Where`, `13:Count`, `28:Open`, `32:Close`, `33:LogMode`, `35:Exec`, `40:HasTable`, `41:Table`, `41:Where`, `41:Count`, `47:HasTable`, `52:Table`, `52:Where`, `52:Count`
- `models/models.go`: `33:Close`, `41:DB`, `180:Save`, `251:Open`, `263:LogMode`, `264:SetLogger`, `277:DB`, `278:DB`, `280:DB`, `287:DB`, `301:Model`, `301:Count`, `317:Save`
- `models/page.go`: `80:Where`, `80:Find`, `91:Where`, `91:Find`, `101:Where`, `101:Find`, `116:Save`, `130:Where`, `130:Save`, `140:Where`, `140:Delete`
- `models/pat.go`: `209:Where`, `209:Order`, `209:Find`, `259:Where`, `259:First`, `285:Model`, `285:Where`, `285:Update`
- `models/privileged.go`: `99:Model`, `99:Where`, `99:Update`, `104:Where`, `104:First`, `111:Save`, `121:Model`, `122:Where`, `122:Count`, `130:Where`, `130:Delete`, `134:Where`, `134:Delete`, `138:Where`, `138:Delete`
- `models/rbac.go`: `79:Where`, `79:First`, `87:Model`, `87:Where`, `87:Association`, `87:Find`
- `models/result.go`: `68:Save`, `80:Save`, `93:Save`, `110:Save`, `127:Save`, `139:Save`, `151:Save`, `174:Save`, `200:Table`, `200:Where`, `200:First`, `212:Where`, `212:First`
- `models/reviewer.go`: `142:Where`, `142:Find`, `148:Model`, `148:Where`, `148:Update`, `194:Preload`, `194:Where`, `195:Order`, `195:Find`, `235:Model`, `239:Joins`, `240:Where`, `242:Where`, `244:Order`, `244:Find`
- `models/secrets_rotation.go`: `50:Table`, `50:Select`, `50:Where`, `50:Scan`, `90:Table`, `90:Select`, `90:Where`, `90:Scan`, `111:Table`, `111:Where`, `111:Update`
- `models/security_repositories.go`: `49:Where`, `49:First`, `55:Model`, `55:Where`, `55:Count`, `63:Where`, `63:First`, `68:Where`, `68:Delete`, `71:Save`, `75:Where`, `75:Delete`, `78:Save`, `82:Model`, `82:Where`, `83:Updates`, `87:Model`, `87:Where`, `88:Updates`, `94:Save`, `98:Model`, `98:Where`, `98:Update`, `112:Where`, `112:First`, `118:Table`, `119:Select`, `120:Joins`, `121:Joins`, `122:Where`, `122:Order`, `122:Scan`, `126:Save`, `129:Where`, `129:Delete`, `135:Model`, `136:Where`, `136:Count`, `141:Begin`, `146:Rollback`, `149:Commit`
- `models/smtp.go`: `83:Save`, `185:Where`, `185:Find`, `194:Where`, `194:Find`, `206:Where`, `206:Find`, `214:Where`, `214:Find`, `225:Where`, `225:Find`, `233:Where`, `233:Find`, `255:Save`, `272:Where`, `277:Where`, `277:Delete`, `285:Save`, `298:Where`, `298:Delete`, `303:Where`, `303:Delete`
- `models/template.go`: `62:Where`, `62:Find`, `69:Where`, `69:Find`, `84:Where`, `84:Find`, `91:Where`, `91:Find`, `105:Where`, `105:Find`, `112:Where`, `112:Find`, `129:Save`, `138:Save`, `154:Where`, `154:Delete`, `164:Save`, `172:Where`, `172:Save`, `184:Where`, `184:Delete`, `191:Where`, `191:Delete`
- `models/user.go`: `35:Preload`, `35:Where`, `35:First`, `42:Preload`, `42:Find`, `50:Preload`, `50:Where`, `50:First`, `56:Save`, `64:Save`, `68:Where`, `68:Delete`, `91:Model`, `91:Where`, `91:Count`, `177:Where`, `177:Delete`
- `models/webhook.go`: `52:Save`, `67:Find`, `79:Where`, `79:Find`, `92:Where`, `92:First`, `127:Where`, `127:Delete`

## Verification log

- Initial protected SHA and zero-open CodeQL baseline verified.
- Inventory and contract-test phase precedes all runtime ORM replacements.
- New shared SQLite contract passes on GORM v1. Full local Go tests, vet,
  staticcheck, trimpath build, vulnerability analysis (zero reachable/imported
  findings), frontend build/audit, 22 Node tests, actionlint and changelog pass.
  Hosted strict MySQL/PostgreSQL contract runs are pending at draft creation.
- Migration, v0.4 fixtures, final hosted matrices/reviews and release remain
  pending; this document does not claim readiness until those results exist.

### Characterization and connection stage

- Draft engineering PR #13, initial head `a35ec6d0c810be72f3a7b6aa304d71ee2c7525d5`:
  CI `34032156613` and CodeQL `34032156599` passed all ten required checks,
  including the unchanged GORM v1 contract on strict MySQL and PostgreSQL.
- `internal/persistence` introduces the maintained adapter/connection factory
  independently; the application still uses v1 until its deliberate switch.
  Its tests prove empty schema on open, pool defaults/overrides, close/health,
  global-write rejection, redacted connection errors and pgx verify-full policy.
- `compatibilitytest` is ORM-independent. CI compiles its seed phase against
  exact immutable v0.4 source and its check phase against the candidate. It
  compares schema and all 29 fixture tables before/after startup, preserves
  ciphertext, exercises login/PAT/campaign/reviewer/reauth/reveal/audit/signed
  exports, performs a normal write, then closes/reopens and verifies again.
  A real v0.4 SQLite seed and unchanged v1 candidate check passed locally.
- Dependency review: pgx v5.10.0 and its pgpass/pgservice/puddle dependencies,
  GORM's `jinzhu/now`, and test-only kr/pretty, kr/text, go-internal additions
  follow the selected maintained modules. `x/net` merely moved to the direct
  section after tidy (already imported by the 0.4 import parser). Existing
  SQLite/MySQL/Goose, crypto, frontend and workflow versions did not change.

### Runtime migration stage

- The independent connection/compatibility stage at `1b9e1fe5436e1aea7525b8723ec8bbb565f2ec99`
  passed CI `34032939964`, PR CodeQL `34032940014` and full-branch CodeQL
  `34032937863`; the full-branch open-alert API returned an empty list. This
  established the two-process v0.4 fixture on all three databases before the
  application switch, not final v2 migration sign-off.
- Application and migration tests now use only GORM v2. Tidy removes both
  `github.com/jinzhu/gorm` and `github.com/lib/pq` from the module graph;
  PostgreSQL SQL-driver registration is pgx v5.10.0. There is no dual routing.
- Scalar lookups use Take/First; collection reads retain successful empty
  results. HTTP adapters use the model-level ErrRecordNotFound alias with
  errors.Is. Scalar campaign/group summaries now fail before reading aggregate
  data when the owner-scoped record is missing.
- Campaign DTO relationships and manually written template attachments/SMTP
  headers are excluded from automatic association persistence. User.Role is
  read-only and remains preloadable. Permission lookup is an explicit bound
  join. Group-target rows without a model primary key use Create, not Save.
- Reused campaign-statistics and audit-filter queries branch from fresh
  sessions. Every aggregate error is propagated. Event sequence reads have
  explicit time/ID ordering. No production AllowGlobalUpdate or AutoMigrate is
  introduced; queue unlocking has an explicit processing=true predicate.
- All nine hooks retain their invariants with the v2 transaction parameter.
  Group/template/page/SMTP BeforeSave supplies a missing modification date;
  IMAP also normalizes nullable login time. User save/find normalizes zero login
  to NULL; campaign find normalizes optional dates. Save invokes hooks inside
  its write transaction. Explicit map/column updates keep their supplied
  false/zero/empty/NULL values; they do not rely on a zero-valued callback model
  to populate map fields. Summary projections still normalize optional dates
  explicitly. None of these hooks grants authority or performs a second write.
- Security transactions use v2 Transaction, preserving original callback and
  commit errors with deferred rollback on panic. Existing manual campaign,
  group, mail-lock and audit transactions now also have deterministic cleanup;
  group delete errors and mail-lock commit errors are returned immediately.
- IMAP's keyless table also uses an explicit insert. Settings are encrypted
  before an atomic owner-scoped delete+insert, so replacement failure cannot
  discard the previous configuration. The shared hook contract covers its
  replacement, booleans, empty optional domain, NULL login and protected secret.
- Shared tests expand to nested-savepoint rollback, panic propagation,
  cancellation-before-commit rollback, failed existing-PAT revocation, reviewer
  assignment, account/role+grant rollback, ciphertext-write rollback, and failed
  then successful wrapping-key rotation. Campaign creation compares hashes of
  all dependency rows, including encrypted SMTP bytes, before/after the write.
- A fresh copy of the actual v0.4 SQLite fixture passes the v2 candidate check:
  schema and all 29 tables unchanged on startup, legacy authentication and
  encrypted review intact, signed audit verification, normal write and restart.
  The original seed remains unchanged. Hosted v2 matrices and reviews remain
  required before readiness; no release is claimed by this stage.
- Full local Go tests, vet, staticcheck, trimpath build and vulnerability scan
  passed after the runtime switch (zero reachable/imported-package findings;
  only the pre-existing unused openpgp module advisory). Frontend lockfile/build/
  audit and clean-output verification, 22 automation tests, changelog validation,
  actionlint and formatting/whitespace checks passed. Windows race execution is
  unavailable in this environment; hosted Linux race remains authoritative.
- Runtime head `0e8704a90a0aac729b4eba68a0c01f342475eb76` passed hosted
  PostgreSQL security/compatibility, SQLite compatibility, Linux race, Go checks,
  workflow checks, frontend, secret scan and full-branch CodeQL. MySQL's new
  association snapshot helper initially omitted quoting the reserved `groups`
  identifier; the helper was corrected without changing strict mode or queries
  in the application. Its rerun remains required.
- The actual v0.4 fixture has one plaintext SMTP setting after its automatic
  campaign association save. The v2 candidate intentionally preserves existing
  bytes on startup; it prevents future association rewrites and the existing
  `secrets status` / `secrets migrate` commands detect and explicitly encrypt
  such legacy values. Operators should inspect and migrate existing plaintext
  integration settings with the configured key provider, after taking a backup;
  startup does not silently rotate them. No real deployment was accessed.
- Native-process smoke preparation discovered that the pre-existing CLI had
  subcommands but no default server command, making bare startup print help.
  A default `serve` command restores the documented launch behavior, with
  parsing tests that keep administrative commands distinct. This is necessary
  for the requested real-binary persistence/restart scenario, not a new product
  capability. Release smoke must still run on the downloaded final artifact.
- The locally built candidate passed the native-process smoke after that fix:
  v0.4 database copy, explicit legacy secret migration, invalid-token denial,
  PAT authentication, campaign read/completion, persisted audit verification,
  process restart/readback and signed export verification. The server bound
  only loopback with the mailer disabled and synthetic accounts/values.
- Head `855fc0f6af0623874f24a8a955e226529514e228` passed all ten required
  hosted checks (CI `34035823736`, PR CodeQL `34035823730`), including strict
  MySQL and PostgreSQL security/compatibility. Full-branch CodeQL `34035824043`
  uploaded both exact-head categories with no errors/warnings and zero open
  alerts. The normal review identified one P2 startup regression: permanent CA
  errors entered the transient retry loop. Immediate rejection is restored,
  with missing/invalid-PEM startup tests. Final-head rechecks and both review
  completions are still mandatory before protected merge.

### Post-readiness PostgreSQL trust follow-up

- Engineering head `ddef190` passed all ten checks, zero-open CodeQL, normal
  review and explicit security review before PR #13 was marked ready. It merged
  through protections as `f784a5d`; that exact main independently passed all ten
  checks and the zero-open baseline. Generated-only release PR #14 exactly
  matched the trusted generator and merged as `26b1cc6`.
- The additional normal review triggered by marking #13 ready subsequently
  identified the same permanent-CA delay for structured PostgreSQL settings.
  The expanded actual-Setup regression test fails on the pre-fix code under a
  five-second timeout, proving entry into the retry sleep. A narrow protected
  follow-up validates the configured PostgreSQL CA with the same parser as
  MySQL, returning redacted ErrTrust before opening; neither TLS verification
  nor transient database retries is weakened. No schema/dependency change.
- This later finding is not treated as resolved by the earlier clean review.
  The follow-up requires its own checks/reviews and regenerated release material
  before publication can be considered verified.
