import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"

const script = readFileSync(new URL("./release-recover.mjs", import.meta.url), "utf8")
const workflow = readFileSync(new URL("../.github/workflows/release-recover.yml", import.meta.url), "utf8")

test("release recovery remains fail closed and source-bound", () => {
  assert.match(script, /github-actions\[bot\]/)
  assert.match(script, /actor\?\.id === 41898282/)
  assert.match(script, /release\.target_commitish/)
  assert.match(script, /darkphish-release-source:/)
  assert.match(script, /compare\/\$\{source\}\.\.\.\$\{main\}/)
  assert.match(script, /pending release source required checks are not green/)
  assert.match(script, /verifyReleaseMaintainerReview/)
  assert.match(script, /verifyCodeQLBaseline/)
  assert.match(script, /generatedPath/)
  assert.match(script, /asset bytes do not match GitHub digest metadata/)
  assert.match(script, /checksum manifest does not match uploaded assets/)
  assert.match(script, /darkphish-release-publication-receipt\/v1/)
  assert.match(script, /protected main changed during release recovery/)
  assert.match(script, /draft: false/)
})

test("release recovery is main-only, serialized, and not manually dispatchable", () => {
  assert.match(workflow, /workflow_run:/)
  assert.match(workflow, /branches: \[main\]/)
  assert.match(workflow, /schedule:/)
  assert.doesNotMatch(workflow, /\bworkflow_dispatch\s*:/)
  assert.match(workflow, /github\.ref == 'refs\/heads\/main'/)
  assert.match(workflow, /group: protected-main-mutation/)
  assert.match(workflow, /cancel-in-progress: false/)
  assert.match(workflow, /contents: write/)
  assert.match(workflow, /persist-credentials: false/)
  assert.match(workflow, /node scripts\/release-recover\.mjs/)
})
