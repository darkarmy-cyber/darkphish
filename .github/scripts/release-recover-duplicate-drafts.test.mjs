import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"

const recovery = readFileSync(new URL("./release-recover.mjs", import.meta.url), "utf8")
const guard = readFileSync(new URL("./release-missing-tag-guard.mjs", import.meta.url), "utf8")

test("duplicate recovery authenticates one immutable source without current-main CodeQL rebinding", () => {
  const state = recovery.slice(recovery.indexOf("async function recoveryState"), recovery.indexOf("function localArtifacts"))
  assert.doesNotMatch(state, /multiple pending release drafts require manual investigation/)
  assert.match(state, /const draftSources = new Set\(drafts\.map\(\(draft\) => draft\?\.target_commitish\)\)/)
  assert.match(state, /draftSources\.size !== 1/)
  assert.match(state, /pending release drafts disagree on one immutable source/)
  assert.match(state, /await verifySourceAncestry\(repo, source, main\); await verifiedReleasePR\(repo, source, version, tag\)/)
  assert.match(state, /const originalRun = await verifyOriginalNativeRelease\(repo, source\)/)
  assert.match(state, /for \(const draft of drafts\) assertExactDraft\(draft, version, source, expected\)/)
  assert.doesNotMatch(state, /verifyCodeQLBaseline\(repo, source\)/)
  assert.match(state, /return \{ tag, source, drafts, originalRun/)
})

test("historical CodeQL and Native-release provenance stay time-bound to the immutable source", () => {
  const original = recovery.slice(recovery.indexOf("async function verifyOriginalNativeRelease"), recovery.indexOf("async function expectedMetadata"))
  assert.match(original, /actions\/runs\?head_sha=\$\{source\}/)
  assert.match(original, /run\.path === "\.github\/workflows\/ci\.yml"/)
  assert.match(original, /run\.path === "\.github\/workflows\/codeql\.yml"/)
  assert.match(original, /run\.head_sha === source/)
  assert.match(original, /Date\.parse\(item\.updated_at\) > Date\.parse\(run\.created_at\)/)
  assert.match(original, /historical release provenance has no qualifying failed Native release run/)
})

test("protected main remains independently pinned to immutable recovery execution SHA", () => {
  const currentMain = recovery.slice(recovery.indexOf("async function currentProtectedMain"), recovery.indexOf("async function readTagState"))
  assert.match(currentMain, /expectedSHA !== null && source !== expectedSHA/)
  assert.match(currentMain, /verifyCodeQLBaseline\(repo, source\)/)
  const publish = recovery.slice(recovery.indexOf("async function publish"), recovery.indexOf("const command ="))
  assert.match(publish, /expectedMain !== executionSHA/)
  const checks = publish.match(/currentProtectedMain\(repo, executionSHA\)/g) || []
  assert.ok(checks.length >= 4)
})

test("duplicate staging drafts are never deleted or used as artifact inputs", () => {
  const publish = recovery.slice(recovery.indexOf("async function publish"), recovery.indexOf("const command ="))
  assert.doesNotMatch(publish, /method: "DELETE"/)
  assert.match(publish, /const historicalDraftIDs = \(state\.drafts \|\| \[\]\)\.map/)
  assert.match(publish, /const verifyStagingSet = async/)
  assert.match(publish, /for \(const candidate of tagged\) assertExactDraft\(candidate, version, source, state\.expected\)/)
  assert.match(publish, /release staging set changed during recovery/)
  assert.match(publish, /let release = await api\(`repos\/\$\{repo\}\/releases`, \{ method: "POST"/)
  assert.ok(publish.indexOf("await verifyStagingSet(state.tagState)") < publish.indexOf("await ensureTag(repo, tag, source, initialTagExpectation)"))
  assert.ok(publish.indexOf("await verifyStagingSet(immutableTag)") < publish.indexOf("let release = await api"))
  assert.ok(publish.indexOf("await verifyStagingSet(immutableTag, release)") < publish.indexOf("for (const name of local.names) await uploadAsset"))
})

test("tagless guard does not mutate or classify private draft staging state", () => {
  assert.doesNotMatch(guard, /reconcileDuplicateDrafts|assertTrustedDraft|method: "DELETE"/)
  assert.match(guard, /release\?\.tag_name === tag && release\.draft === false/)
})
