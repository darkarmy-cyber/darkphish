import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"

const script = readFileSync(new URL("./release-recover.mjs", import.meta.url), "utf8")
const workflow = readFileSync(new URL("../workflows/release-recover.yml", import.meta.url), "utf8")

test("recovery metadata and publication stay bound to canonical source metadata", () => {
  assert.match(script, /notesForVersion/)
  assert.match(script, /releaseName/)
  assert.match(script, /releaseBody/)
  assert.match(script, /pending release draft metadata does not exactly match the verified source/)
  assert.match(script, /published release metadata does not exactly match the verified source/)
  assert.match(script, /verifyReleaseMaintainerReview/)
  assert.match(script, /generatedPath/)
})

test("historical release provenance is exact-SHA and canonical-workflow bound", () => {
  const historical = script.slice(script.indexOf("async function verifyOriginalNativeRelease"), script.indexOf("async function expectedMetadata"))
  assert.match(historical, /run\.path === "\.github\/workflows\/ci\.yml"/)
  assert.match(historical, /run\.path === "\.github\/workflows\/codeql\.yml"/)
  assert.match(historical, /Native release/)
  assert.match(historical, /Run node scripts\/release-publish\.mjs metadata/)
  assert.match(historical, /Generate checksums/)
  assert.match(historical, /historical release provenance has no qualifying failed Native release run/)
  assert.match(historical, /historical release provenance spans unexpected Native release workflows/)
})

test("protected main verification can be pinned to immutable execution SHA", () => {
  const fn = script.slice(script.indexOf("async function currentProtectedMain"), script.indexOf("async function readTagState"))
  assert.match(fn, /expectedSHA !== null && source !== expectedSHA/)
  assert.match(fn, /verifyCodeQLBaseline\(repo, source\)/)
  const greens = fn.match(/greenCommit\(repo, source\)/g) || []
  assert.ok(greens.length >= 2)
  assert.match(fn, /protected main changed after final required-check verification/)
})

test("metadata revalidates immutable execution SHA and exact precheck tag snapshot", () => {
  const metadata = script.slice(script.indexOf("async function metadata"), script.indexOf("async function publish"))
  assert.match(metadata, /RECOVERY_EXECUTION_SHA \|\| process\.env\.GITHUB_SHA/)
  assert.match(metadata, /currentProtectedMain\(repo, executionSHA\)/)
  assert.match(metadata, /parsePrecheckSnapshot\(process\.env\.RECOVERY_PRECHECK_TAG_ABSENT\)/)
  assert.match(metadata, /assertPrecheckSnapshot\(repo, tag, precheck/)
  assert.match(metadata, /output\("main", finalMain\)/)
})

test("present-tag precheck preserves exact object type and SHA", () => {
  assert.match(script, /\^false:\(commit\|tag\):\(\[a-f0-9\]\{40\}\)\$/)
  assert.match(script, /current\.objectType !== snapshot\.objectType/)
  assert.match(script, /current\.objectSha !== snapshot\.objectSha/)
})

test("published recovery attests every expected asset including receipt", () => {
  const qualifying = script.slice(script.indexOf("async function qualifyingRecoveryRun"), script.indexOf("async function verifyPublishedRecovery"))
  assert.match(qualifying, /const attestedNames = expectedReleaseAssetNames\(version\)\.sort\(\)/)
  assert.doesNotMatch(qualifying, /\.filter\(\(name\) => name !== receiptName\)/)
  assert.match(qualifying, /for \(const name of attestedNames\) verifyRecoveryAttestation/)
})

test("recovery attestation is bound to exact workflow run and attempt", () => {
  assert.match(script, /"--format", "json"/)
  assert.match(script, /predicate\?\.runDetails\?\.metadata\?\.invocationId/)
  assert.match(script, /actions\/runs\/\$\{run\.id\}\/attempts\/\$\{run\.run_attempt\}/)
  assert.match(script, /recovery attestation is not bound to the selected workflow run and attempt/)
})

test("published recovery is reread after final protected-main verification", () => {
  const verify = script.slice(script.indexOf("async function verifyPublishedRecovery"), script.indexOf("async function ensureDraftState"))
  const snapshots = verify.match(/await verifySnapshot\(\)/g) || []
  assert.ok(snapshots.length >= 2)
  assert.match(verify, /currentProtectedMain\(repo, process\.env\.RECOVERY_EXECUTION_SHA \|\| process\.env\.GITHUB_SHA\)/)
  assert.match(verify, /await verifySourceAncestry\(repo, run\.head_sha, finalMain\)/)
})

test("rollback uses stable direct-ID, list and by-tag snapshot", () => {
  const fn = script.slice(script.indexOf("async function ensureDraftState"), script.indexOf("async function withdrawPublishedRelease"))
  assert.match(fn, /const ids = new Set\(\[releaseId\]\)/)
  assert.match(fn, /if \(Number\.isSafeInteger\(byTag\?\.id\)\) ids\.add\(byTag\.id\)/)
  assert.match(fn, /const finalDirect = await Promise\.all/)
  assert.match(fn, /release\.draft !== true \|\| release\.prerelease !== false/)
  assert.match(fn, /publicAfter\.length === 0/)
  assert.match(fn, /assertTagSnapshot/)
})

test("publish refuses any main value not equal to immutable execution SHA", () => {
  const publish = script.slice(script.indexOf("async function publish"), script.indexOf("const command ="))
  assert.match(publish, /expectedMain !== executionSHA/)
  assert.match(publish, /currentProtectedMain\(repo, executionSHA\)/)
  const checks = publish.match(/currentProtectedMain\(repo, executionSHA\)/g) || []
  assert.ok(checks.length >= 4)
})

test("publish verifies exact tag and rebuilt asset bytes before and after publication", () => {
  const publish = script.slice(script.indexOf("async function publish"), script.indexOf("const command ="))
  assert.match(publish, /ensureTag\(repo, tag, source, initialTagExpectation\)/)
  assert.match(publish, /assertUploadedAssetSet/)
  assert.match(publish, /assertTagState\(repo, tag, immutableTag/)
  assert.match(publish, /verifyPublishedSnapshot/)
  const snapshots = publish.match(/await verifyPublishedSnapshot\(\)/g) || []
  assert.ok(snapshots.length >= 3)
})

test("ambiguous publication always enters confirmed rollback", () => {
  const publish = script.slice(script.indexOf("async function publish"), script.indexOf("const command ="))
  assert.match(publish, /publication PATCH returned an ambiguous failure/)
  assert.match(publish, /withdrawPublishedRelease\(repo, release\.id, tag, immutableTag/)
})

test("recovery rebuilds immutable source and mandates receipt-inclusive attestations", () => {
  assert.match(workflow, /Rebuild and package immutable release source/)
  assert.match(workflow, /Generate recovery checksums/)
  assert.match(workflow, /Generate recovery publication receipt/)
  assert.match(workflow, /name: Attest rebuilt recovery artifacts\n\s+uses: actions\/attest@v4/)
  assert.match(workflow, /subject-path:\s*\|[\s\S]*?dist\/\*[\s\S]*?\.cache\/recovery-receipt\/\*/)
  assert.match(workflow, /id-token: write/)
  assert.match(workflow, /attestations: write/)
})

test("recovery workflow is serialized main-only automation", () => {
  assert.match(workflow, /workflow_run:/)
  assert.match(workflow, /branches: \[main\]/)
  assert.match(workflow, /schedule:/)
  assert.doesNotMatch(workflow, /\bworkflow_dispatch\s*:/)
  assert.match(workflow, /group: protected-main-mutation/)
  assert.match(workflow, /cancel-in-progress: false/)
  assert.match(workflow, /node \.github\/scripts\/release-recover\.mjs metadata/)
  assert.match(workflow, /node \.github\/scripts\/release-recover\.mjs publish/)
})
