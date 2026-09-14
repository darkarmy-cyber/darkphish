import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync } from "node:fs"

const script = readFileSync(new URL("./release-draft-tag-repair.mjs", import.meta.url), "utf8")

test("detached draft repair is narrowly scoped and fail closed", () => {
  assert.match(script, /const detachedTag = \/\^untagged-/)
  assert.match(script, /candidates\.length > 1/)
  assert.match(script, /multiple detached canonical release drafts require manual forensic review/)
  assert.match(script, /release\.draft !== true/)
  assert.match(script, /release\.prerelease !== false/)
  assert.match(script, /historical rollback state/)
})

test("repair distinguishes stale detached aliases from real canonical conflicts", () => {
  assert.match(script, /releases\/tags\/\$\{tag\}/)
  assert.match(script, /canonical && canonical\.id !== summary\.id/)
  assert.match(script, /function staleDetachedAlias/)
  assert.match(script, /release\.tag_name !== tag/)
  assert.match(script, /release\.target_commitish !== source/)
  assert.match(script, /release\.name !== recoveryName/)
  assert.match(script, /url\.hostname !== "github\.com"/)
  assert.match(script, /detachedTag\.test\(url\.pathname\.slice\(prefix\.length\)\)/)
  assert.match(script, /conflicting release identity requires manual forensic review/)
})

test("repair revalidates immutable tag, canonical body and exact assets", () => {
  assert.match(script, /await tagCommit\(repo, tag\) !== source/)
  assert.match(script, /releaseBody\(changelog, version, source\)/)
  assert.match(script, /release\.body !== expectedBody/)
  assert.match(script, /expectedReleaseAssetNames\(version\)/)
  assert.match(script, /assertSameAssets\(closingAssets, snapshot\)/)
})

test("repair explicitly restores only release metadata while preserving state", () => {
  assert.match(script, /tag_name: tag/)
  assert.match(script, /target_commitish: source/)
  assert.match(script, /name: release\.name/)
  assert.match(script, /body: release\.body/)
  assert.match(script, /draft: true/)
  assert.match(script, /prerelease: false/)
  assert.doesNotMatch(script, /DELETE/)
})
