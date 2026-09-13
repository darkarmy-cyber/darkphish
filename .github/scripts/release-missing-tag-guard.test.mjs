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
  assert.match(guard, /const direct = await Promise\.all/)
  assert.match(guard, /directNotWithdrawn/)
  assert.match(guard, /release\.draft !== true \|\| release\.prerelease !== false/)
  assert.match(guard, /publicByTag/)
  assert.match(guard, /output\("tag_absent", "true"\)/)
})
