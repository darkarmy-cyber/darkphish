import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync } from "node:fs"

const workflow = readFileSync(new URL("../workflows/release-recover.yml", import.meta.url), "utf8")
const guard = readFileSync(new URL("./release-publication-guard.mjs", import.meta.url), "utf8")

test("recovery executable checkout is pinned to the immutable workflow run SHA", () => {
  const pinned = workflow.match(/uses: actions\/checkout@v7\n\s+with:\n\s+ref: \$\{\{ github\.sha \}\}/g) || []
  assert.ok(pinned.length >= 2, "metadata and publish executable checkouts must both use github.sha")
  assert.match(workflow, /RECOVERY_EXECUTION_SHA: \$\{\{ github\.sha \}\}/)
  assert.match(workflow, /Verify immutable recovery execution before any release mutation/)
  assert.match(workflow, /release-publication-guard\.mjs execution/)
  assert.match(guard, /branch\?\.commit\?\.sha !== expectedSHA/)
  assert.match(guard, /protected main moved during recovery execution verification/)
})

test("publication receipt is generated before and included in recovery attestation", () => {
  const receipt = workflow.indexOf("Generate recovery publication receipt")
  const attest = workflow.indexOf("Attest rebuilt recovery artifacts")
  const publish = workflow.indexOf("Publish rebuilt verified recovery assets")
  assert.ok(receipt > 0 && receipt < attest && attest < publish)
  assert.match(workflow, /subject-path: \|[\s\S]*dist\/\*[\s\S]*\.cache\/recovery-receipt\/\*/)
  assert.match(guard, /for \(const asset of common\.assets\) verifyAttestation/)
  assert.doesNotMatch(guard, /expectedReleaseAssetNames\(version\)\.filter\([^\n]*receipt/)
})

test("public release is preserved only after recovery or canonical native provenance verification", () => {
  assert.match(guard, /verifyRecoveryPublication\(/)
  assert.match(guard, /verifyNativePublication\(/)
  assert.match(guard, /\.github\/workflows\/release-recover\.yml/)
  assert.match(guard, /\.github\/workflows\/release\.yml/)
  assert.match(guard, /failed both trusted publication paths and will be withdrawn/)
  assert.match(guard, /output\("verified", "true"\); output\("kind", "native"\)/)
  assert.match(guard, /output\("verified", "true"\); output\("kind", "recovery"\)/)
})

test("native preservation requires successful attestations for every published asset and exact run attempt", () => {
  assert.match(guard, /attestStep\?\.status !== "completed" \|\| attestStep\.conclusion !== "success"/)
  assert.match(guard, /canonical Native release did not attest its complete published asset set/)
  assert.match(guard, /for \(const asset of common\.assets\) verifyAttestation\(repo, local\.get\(asset\.name\)\.path, "\.github\/workflows\/release\.yml", common\.source, run\)/)
  assert.match(guard, /"--format", "json"/)
  assert.match(guard, /predicate\?\.runDetails\?\.metadata\?\.invocationId/)
  assert.match(guard, /actions\/runs\/\$\{run\.id\}\/attempts\/\$\{run\.run_attempt\}/)
  assert.doesNotMatch(guard, /attestStep\?\.conclusion !== "skipped"/)
})

test("unverified publication withdrawal uses a stable direct release and by-tag confirmation", () => {
  assert.match(guard, /const finalDirect = await Promise\.all/)
  assert.match(guard, /releases\/\$\{id\}/)
  assert.match(guard, /releases\/tags\/\$\{tag\}/)
  assert.match(guard, /if \(Number\.isSafeInteger\(byTag\?\.id\)\) ids\.add\(byTag\.id\)/)
  assert.match(guard, /directNotWithdrawn/)
  assert.match(guard, /release\.draft !== true \|\| release\.prerelease !== false/)
  assert.match(guard, /publicByTag/)
  assert.match(guard, /release tag appeared while confirming withdrawal of an explicitly tagless publication/)
})

test("tag precheck carries the exact present tag object and cannot be silently re-baselined", () => {
  assert.match(workflow, /RECOVERY_PRECHECK_TAG_ABSENT: \$\{\{ steps\.missing_tag\.outputs\.tag_absent \}\}/)
  assert.match(workflow, /grep -qx 'tag_present=true'/)
  assert.match(guard, /\^false:\(commit\|tag\):\(\[a-f0-9\]\{40\}\)\$/)
  assert.match(guard, /tagState\.objectType !== precheck\.tagState\.objectType/)
  assert.match(guard, /tagState\.objectSha !== precheck\.tagState\.objectSha/)
  assert.match(guard, /release tag appeared after the explicit absent-tag precheck/)
  assert.match(guard, /release tag disappeared after the tagged precheck/)
  assert.match(guard, /release tag object changed after the tagged precheck/)
  assert.match(guard, /tagless public release appeared after the absent-tag precheck/)
})

test("all trusted paths close with release, tag, assets, and protected-main TOCTOU rechecks", () => {
  const snapshots = guard.match(/await assertPublishedSnapshot\(repo, release, version, tagState, common\)/g) || []
  assert.ok(snapshots.length >= 4, "both native and recovery paths must snapshot before and after final main verification")
  assert.match(guard, /assertSameAssets\(await pages\(`repos\/\$\{repo\}\/releases\/\$\{release\.id\}\/assets`\), common\.snapshot\)/)
  assert.match(guard, /assertTagState\(repo, versionTag\(version\), tagState/)
  const finalMainChecks = guard.match(/await executionMain\(repo, process\.env\.RECOVERY_EXECUTION_SHA\)/g) || []
  assert.ok(finalMainChecks.length >= 2)
})
