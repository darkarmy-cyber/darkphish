import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync } from "node:fs"

const workflow = readFileSync(new URL("../workflows/release-recover.yml", import.meta.url), "utf8")
const guard = readFileSync(new URL("./release-publication-guard.mjs", import.meta.url), "utf8")

test("recovery executable checkout is pinned to the immutable workflow run SHA", () => {
  const pinned = workflow.match(/uses: actions\/checkout@v7\n\s+with:\n\s+ref: \$\{\{ github\.sha \}\}/g) || []
  assert.ok(pinned.length >= 2, "metadata and publish executable checkouts must both use github.sha")
  assert.match(workflow, /RECOVERY_EXECUTION_SHA: \$\{\{ github\.sha \}\}/)
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

test("native attestations are required exactly when the canonical workflow emitted them", () => {
  assert.match(guard, /attestStep\?\.conclusion === "success"/)
  assert.match(guard, /verifyAttestation\(repo, local\.get\(asset\.name\)\.path, "\.github\/workflows\/release\.yml", common\.source\)/)
  assert.match(guard, /attestStep\?\.conclusion !== "skipped"/)
})

test("all trusted paths close with tag, assets, and protected-main TOCTOU rechecks", () => {
  assert.match(guard, /assertSameAssets\(await pages\(`repos\/\$\{repo\}\/releases\/\$\{release\.id\}\/assets`\), common\.snapshot\)/)
  assert.match(guard, /assertTagState\(repo, versionTag\(version\), tagState/)
  const finalMainChecks = guard.match(/await executionMain\(repo, process\.env\.RECOVERY_EXECUTION_SHA\)/g) || []
  assert.ok(finalMainChecks.length >= 2)
})
