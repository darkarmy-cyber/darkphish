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
  assert.match(script, /historical release provenance does not map to exactly one failed Native release run/)
  assert.doesNotMatch(script, /verifyCodeQLBaseline\(repo, source\)/)
  assert.match(script, /verifyCodeQLBaseline\(repo, branch\.commit\.sha\)/)
})

test("recovery rebuilds instead of adopting staged binaries", () => {
  assert.match(workflow, /Rebuild and package immutable release source/)
  assert.match(workflow, /ref: \$\{\{ needs\.metadata\.outputs\.source \}\}/)
  assert.match(workflow, /recovery-\$\{\{ needs\.metadata\.outputs\.tag \}\}-\$\{\{ matrix\.goos \}\}-\$\{\{ matrix\.goarch \}\}/)
  assert.match(workflow, /Generate recovery checksums/)
  assert.match(workflow, /audit-binary-smoke/)
  assert.match(script, /rebuilt recovery artifact set is incomplete or unexpected/)
  assert.match(script, /verifyChecksums/)
})

test("recovery makes the tag immutable before publication and verifies publication afterwards", () => {
  assert.match(script, /repos\/\$\{repo\}\/git\/refs/)
  assert.match(script, /refs\/tags\/\$\{tag\}/)
  assert.match(script, /release tag does not resolve to the immutable verified source/)
  assert.match(script, /release tag changed immediately before publication/)
  assert.match(script, /repos\/\$\{repo\}\/releases\/tags\/\$\{tag\}/)
  assert.match(script, /post-publication release metadata verification failed/)
  assert.match(script, /post-publication tag verification failed/)
  assert.match(script, /assertCurrentVersionPublished/)
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
