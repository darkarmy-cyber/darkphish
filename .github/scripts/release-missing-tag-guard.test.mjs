import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync } from "node:fs"

const workflow = readFileSync(new URL("../workflows/release-recover.yml", import.meta.url), "utf8")
const guard = readFileSync(new URL("./release-missing-tag-guard.mjs", import.meta.url), "utf8")

test("tagless public releases are withdrawn before publication provenance preflight", () => {
  const missingTagGuard = workflow.indexOf("Withdraw tagless public release fail closed")
  const publicationGuard = workflow.indexOf("Preserve only cryptographically verified public releases")
  assert.ok(missingTagGuard > 0 && publicationGuard > missingTagGuard)
  assert.match(workflow, /node \.github\/scripts\/release-missing-tag-guard\.mjs/)
  assert.match(guard, /release\.tag_name === tag && release\.draft === false/)
  assert.match(guard, /draft: true/)
  assert.match(guard, /if \(!releases\.length && !tagState\)/)
  assert.match(guard, /tag appeared .* fail closed|refusing to establish a new trust baseline/s)
})
