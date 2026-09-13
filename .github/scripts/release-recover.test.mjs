import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"

const script = readFileSync(new URL("./release-recover.mjs", import.meta.url), "utf8")
const workflow = readFileSync(new URL("../workflows/release-recover.yml", import.meta.url), "utf8")

test("recovery binds metadata to the verified source", () => {
  assert.match(script, /notesForVersion/)
  assert.match(script, /releaseName/)
  assert.match(script, /releaseBody/)
  assert.match(script, /pending release draft metadata does not exactly match the verified source/)
  assert.match(script, /published release metadata does not exactly match the verified source/)
  assert.match(script, /darkphish-release-source:/)
  assert.match(script, /verifyReleaseMaintainerReview/)
  assert.match(script, /generatedPath/)
})

test("recovery proves historical release provenance without treating an ancestor as current CodeQL", () => {
  assert.match(script, /historical release source lacks successful exact-SHA CI or CodeQL/)
  assert.match(script, /Native release/)
  assert.match(script, /Run node scripts\/release-publish\.mjs metadata/)
  assert.match(script, /Run actions\/download-artifact@v8/)
  assert.match(script, /Generate checksums/)
  assert.match(script, /historical release provenance has no qualifying failed Native release run/)
  assert.match(script, /candidates\.sort\(\(a, b\) => b\.id - a\.id\)/)
  assert.match(script, /historical release provenance spans unexpected Native release workflows/)
  assert.doesNotMatch(script, /verifyCodeQLBaseline\(repo, source\)/)
  assert.match(script, /verifyCodeQLBaseline\(repo, branch\.commit\.sha\)/)
})

test("recovery rebuilds instead of adopting staged binaries and attests rebuilt bytes", () => {
  assert.match(workflow, /Rebuild and package immutable release source/)
  assert.match(workflow, /ref: \$\{\{ needs\.metadata\.outputs\.source \}\}/)
  assert.match(workflow, /recovery-\$\{\{ needs\.metadata\.outputs\.tag \}\}-\$\{\{ matrix\.goos \}\}-\$\{\{ matrix\.goarch \}\}/)
  assert.match(workflow, /Generate recovery checksums/)
  assert.match(workflow, /audit-binary-smoke/)
  assert.match(workflow, /id-token: write/)
  assert.match(workflow, /attestations: write/)
  assert.match(workflow, /uses: actions\/attest@v4/)
  assert.match(workflow, /subject-path: dist\/\*/)
  assert.match(script, /rebuilt recovery artifact set is incomplete or unexpected/)
  assert.match(script, /verifyChecksums/)
})

test("recovery withdraws every pre-existing public release before trusting recovery state", () => {
  const recovery = script.slice(script.indexOf("async function recoveryState"), script.indexOf("function localArtifacts"))
  assert.match(recovery, /for \(const release of published\) await ensureDraftState\(repo, release\.id, tag\)/)
  assert.match(recovery, /published release state could not be withdrawn before trusted recovery/)
  assert.match(recovery, /recovery will rebuild and attest all assets before publication/)
  assert.doesNotMatch(recovery, /return \{ published: true/)
})

test("metadata remediation has write permission and revalidates protected main", () => {
  const metadataJob = workflow.slice(workflow.indexOf("\n  metadata:"), workflow.indexOf("\n  verify:"))
  assert.match(metadataJob, /permissions:\n\s+contents: write/)
  assert.match(script, /protected main changed during recovery metadata verification/)
  assert.match(script, /await verifySourceAncestry\(repo, state\.source, finalMain\)/)
})

test("recovery pins the exact immutable tag ref object, not only the peeled commit", () => {
  assert.match(script, /async function readTagState/)
  assert.match(script, /objectType/)
  assert.match(script, /objectSha/)
  assert.match(script, /sameTagState/)
  assert.match(script, /release tag ref object changed since recovery metadata verification/)
  assert.match(script, /release tag appeared since recovery metadata verification/)
  assert.match(script, /final immutable release tag ref object verification failed/)
  assert.match(workflow, /RECOVERY_TAG_PRESENT/)
  assert.match(workflow, /RECOVERY_TAG_OBJECT_TYPE/)
  assert.match(workflow, /RECOVERY_TAG_OBJECT_SHA/)
})

test("recovery rollback confirms no replacement public release remains for the tag", () => {
  assert.match(script, /const publicForTag = currentReleases\.filter/)
  assert.match(script, /replacement withdrawal PATCH attempt/)
  assert.match(script, /const publicAfter = after\.filter/)
  assert.match(script, /releases\/tags\/\$\{tag\}/)
  assert.match(script, /publicAfter\.length === 0 && !publicByTag/)
  assert.match(script, /immutable release tag ref object changed while withdrawing publication/)
})

test("recovery treats publication PATCH errors as ambiguous and forces confirmed rollback", () => {
  const publishPatch = 'body: { draft: false, prerelease: false, make_latest: "true" }'
  const patchIndex = script.indexOf(publishPatch)
  const ambiguousIndex = script.indexOf("publication PATCH returned an ambiguous failure")
  const rollbackIndex = script.indexOf("await withdrawPublishedRelease(repo, release.id, tag, immutableTag", patchIndex)
  assert.ok(patchIndex >= 0 && ambiguousIndex > patchIndex && rollbackIndex > patchIndex)
  assert.match(script, /publication PATCH returned an ambiguous failure/)
  assert.match(script, /ensureDraftState/)
})

test("final publication acceptance rebinds fresh assets to locally rebuilt digests", () => {
  const genericIndex = script.indexOf("const fullyVerified = await assertCurrentVersionPublished")
  const finalReleaseIndex = script.indexOf("const finalRelease = assertExactPublished", genericIndex)
  const finalAssetsIndex = script.indexOf("assertUploadedAssetSet(finalAssets, local, receiptHash, receipt.length)", finalReleaseIndex)
  const finalMainIndex = script.indexOf("protected main changed before final recovery publication acceptance", finalAssetsIndex)
  assert.ok(genericIndex >= 0 && finalReleaseIndex > genericIndex && finalAssetsIndex > finalReleaseIndex && finalMainIndex > finalAssetsIndex)
})

test("recovery only deletes an exactly verified stale draft after rebuilt artifacts exist", () => {
  const localIndex = script.indexOf("const local = localArtifacts(version)")
  const deleteIndex = script.indexOf('method: "DELETE"')
  assert.ok(localIndex >= 0 && deleteIndex > localIndex)
  assert.match(script, /stale release draft still exists after deletion/)
  assert.match(script, /release state appeared after stale draft deletion/)
})

test("release recovery is main-only, serialized, and not manually dispatchable", () => {
  assert.match(workflow, /workflow_run:/)
  assert.match(workflow, /branches: \[main\]/)
  assert.match(workflow, /schedule:/)
  assert.doesNotMatch(workflow, /\bworkflow_dispatch\s*:/)
  assert.match(workflow, /group: protected-main-mutation/)
  assert.match(workflow, /cancel-in-progress: false/)
  assert.match(workflow, /persist-credentials: false/)
  assert.match(workflow, /node \.github\/scripts\/release-recover\.mjs metadata/)
  assert.match(workflow, /node \.github\/scripts\/release-recover\.mjs publish/)
})
