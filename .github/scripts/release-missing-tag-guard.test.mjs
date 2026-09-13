import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync } from "node:fs"

const workflow = readFileSync(new URL("../workflows/release-recover.yml", import.meta.url), "utf8")
const guard = readFileSync(new URL("./release-missing-tag-guard.mjs", import.meta.url), "utf8")

test("tagless public releases are withdrawn before publication provenance preflight", () => {
  const executionGuard = workflow.indexOf("Verify immutable recovery execution before any release mutation")
  const missingTagGuard = workflow.indexOf("Withdraw tagless public release fail closed")
  const publicationGuard = workflow.indexOf("Preserve only cryptographically verified public releases")
  assert.ok(executionGuard > 0 && missingTagGuard > executionGuard && publicationGuard > missingTagGuard)
  assert.match(workflow, /node \.github\/scripts\/release-publication-guard\.mjs execution/)
  assert.match(workflow, /node \.github\/scripts\/release-missing-tag-guard\.mjs/)
  assert.match(guard, /release\?\.tag_name === tag && release\.draft === false/)
  assert.match(guard, /draft: true/)
  assert.match(guard, /tag_absent/)
  assert.match(guard, /releases\/tags\/\$\{tag\}/)
  assert.match(guard, /release tag appeared while withdrawing tagless public release/)
})

test("tag appearance never prevents confirmed withdrawal and cannot become a recovery baseline", () => {
  const patch = guard.indexOf('method: "PATCH"')
  const tagProbe = guard.indexOf("await currentTag(repo, tag)")
  const appearedFailure = guard.indexOf("release tag appeared while withdrawing tagless public release")
  assert.ok(patch >= 0 && tagProbe >= 0 && appearedFailure > patch)
  assert.match(guard, /const finalDirect = await directSnapshot/)
  assert.match(guard, /directNotWithdrawn/)
  assert.match(guard, /release\.draft !== true \|\| release\.prerelease !== false/)
  assert.match(guard, /publicByTag/)
  assert.match(guard, /output\("tag_absent", "true"\)/)
})

test("present tag precheck exports exact immutable ref object identity", () => {
  assert.match(guard, /\["commit", "tag"\]\.includes\(objectType\)/)
  assert.match(guard, /`false:\$\{tagState\.objectType\}:\$\{tagState\.objectSha\}`/)
  assert.match(guard, /output\("tag_absent", encodedTagSnapshot\(tagState\)\)/)
})

test("duplicate tagless drafts are collapsed only after exact trusted source and metadata validation", () => {
  const reconcile = guard.slice(guard.indexOf("async function reconcileDuplicateDrafts"), guard.indexOf("async function withdrawWhileTagAbsent"))
  assert.match(reconcile, /const sources = new Set\(drafts\.map\(\(draft\) => draft\?\.target_commitish\)\)/)
  assert.match(reconcile, /sources\.size !== 1/)
  assert.match(reconcile, /pending release drafts disagree on immutable source/)
  assert.match(reconcile, /const expected = await expectedMetadata\(repo, source, version\)/)
  assert.match(reconcile, /for \(const draft of drafts\) assertTrustedDraft\(draft, tag, source, expected\)/)
  assert.match(guard, /actor\?\.id === 41898282/)
  assert.match(guard, /release\.published_at !== null/)
  assert.match(guard, /release\.body !== expected\.body/)
  assert.match(guard, /release\.name !== expected\.name/)
})

test("duplicate cleanup never adopts staged bytes and confirms stable deletion while tag stays absent", () => {
  const reconcile = guard.slice(guard.indexOf("async function reconcileDuplicateDrafts"), guard.indexOf("async function withdrawWhileTagAbsent"))
  const validateAll = reconcile.indexOf("for (const draft of drafts) assertTrustedDraft")
  const deleteLoop = reconcile.indexOf("for (const stale of ordered.slice(1))")
  assert.ok(validateAll >= 0 && deleteLoop > validateAll)
  assert.match(reconcile, /assertTrustedDraft\(await api\(`repos\/\$\{repo\}\/releases\/\$\{stale\.id\}`\), tag, source, expected\)/)
  assert.match(reconcile, /method: "DELETE"/)
  assert.match(reconcile, /duplicate stale release draft still exists after deletion/)
  assert.match(reconcile, /release tag appeared during duplicate draft reconciliation/)
  assert.match(reconcile, /release tag appeared after duplicate draft reconciliation/)
  assert.match(reconcile, /remaining\.length !== 1 \|\| remaining\[0\]\.id !== survivor\.id/)
  assert.doesNotMatch(reconcile, /assets|download|digest|SHA256SUMS/)
})

test("tagless path reconciles drafts even when no public release exists", () => {
  const main = guard.slice(guard.indexOf("async function main"))
  assert.match(main, /if \(releases\.length\) await withdrawWhileTagAbsent\(repo, tag, releases\)/)
  assert.match(main, /await reconcileDuplicateDrafts\(repo, tag, version\)/)
  assert.ok(main.indexOf("await reconcileDuplicateDrafts") < main.indexOf('output("tag_absent", "true")'))
  assert.match(main, /release tag appeared after tagless recovery reconciliation/)
})
