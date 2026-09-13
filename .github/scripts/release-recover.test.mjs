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

test("recovery makes the tag immutable before publication and verifies exact publication metadata afterwards", () => {
  assert.match(script, /repos\/\$\{repo\}\/git\/refs/)
  assert.match(script, /refs\/tags\/\$\{tag\}/)
  assert.match(script, /release tag does not resolve to the immutable verified source/)
  assert.match(script, /release tag changed immediately before publication/)
  assert.match(script, /repos\/\$\{repo\}\/releases\/tags\/\$\{tag\}/)
  assert.match(script, /assertExactPublished\(await api\(`repos\/\$\{repo\}\/releases\/\$\{release\.id\}`/)
  assert.match(script, /assertExactPublished\(fullyVerified, version, source, state\.expected\)/)
  assert.match(script, /post-publication tag verification failed/)
  assert.match(script, /assertCurrentVersionPublished/)
})

test("recovery retries and probes withdrawal until draft state and immutable tag are confirmed", () => {
  assert.match(script, /async function ensureDraftState/)
  assert.match(script, /for \(;;\)/)
  assert.match(script, /withdrawal PATCH attempt/)
  assert.match(script, /withdrawal probe attempt/)
  assert.match(script, /withdrawn\?\.draft === true/)
  assert.match(script, /immutable release tag changed while withdrawing publication/)
  assert.match(script, /await delay\(Math\.min\(5000, 500 \* attempt\)\)/)
  assert.match(script, /release publication was withdrawn after verification failed/)
})

test("recovery treats publication PATCH errors as ambiguous and forces confirmed rollback", () => {
  const publishPatch = 'body: { draft: false, prerelease: false, make_latest: "true" }'
  const patchIndex = script.indexOf(publishPatch)
  const ambiguousIndex = script.indexOf("publication PATCH returned an ambiguous failure")
  const rollbackIndex = script.indexOf("await withdrawPublishedRelease(repo, release.id, tag, source", patchIndex)
  assert.ok(patchIndex >= 0 && ambiguousIndex > patchIndex && rollbackIndex > patchIndex)
  assert.match(script, /publication PATCH returned an ambiguous failure/)
  assert.match(script, /ensureDraftState/)
})

test("published fast path applies the full recovery provenance boundary", () => {
  const fastPath = script.slice(script.indexOf("if (published.length)"), script.indexOf("const drafts = tagged.filter"))
  assert.match(fastPath, /tagCommit\(repo, tag\)/)
  assert.match(fastPath, /verifySourceAncestry\(repo, source, main\)/)
  assert.match(fastPath, /verifiedReleasePR\(repo, source, version, tag\)/)
  assert.match(fastPath, /verifyOriginalNativeRelease\(repo, source\)/)
  assert.match(fastPath, /expectedMetadata\(repo, source, version\)/)
  assert.match(fastPath, /assertExactPublished/)
  assert.match(fastPath, /assertCurrentVersionPublished/)
  assert.match(fastPath, /withdrawPublishedRelease/)
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
  assert.match(workflow, /contents: write/)
  assert.match(workflow, /persist-credentials: false/)
  assert.match(workflow, /node \.github\/scripts\/release-recover\.mjs metadata/)
  assert.match(workflow, /node \.github\/scripts\/release-recover\.mjs publish/)
})
