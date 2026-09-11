import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"
import { assetDisposition, assertGeneratedCommits, assertPublishedVersion, assertReleaseState, checksPassed, generatedPath, nextPatchVersion, protectedMergeRequest, verifyChecksums, versionTag } from "./release-lib.mjs"

test("require every latest trusted check to succeed", () => {
  const check = { id: 1, name: "Go", app: { slug: "github-actions" }, status: "completed", conclusion: "success" }
  assert.equal(checksPassed([check], ["Go"]), true)
  assert.equal(checksPassed([check], ["Go", "Security"]), false)
  assert.equal(checksPassed([check, { ...check, id: 2, status: "in_progress" }], ["Go"]), false)
  assert.equal(checksPassed([{ ...check, app: { slug: "unknown" } }], ["Go"]), false)
  assert.equal(checksPassed([{ ...check, conclusion: "skipped" }], ["Go"]), false)
})
test("generated recovery refuses unknown commits and application files", () => {
  const commits = [{ author: "github-actions[bot]", subject: "release: Darkphish 0.3.0" }]
  assert.doesNotThrow(() => assertGeneratedCommits(commits, ["CHANGELOG.md", "changes/fix.md"], "0.3.0"))
  assert.throws(() => assertGeneratedCommits([{ ...commits[0], author: "developer" }], ["CHANGELOG.md"], "0.3.0"))
  assert.throws(() => assertGeneratedCommits(commits, ["models/user.go"], "0.3.0"))
  assert.equal(generatedPath("changes/../models/user.go"), false)
  assert.throws(() => versionTag("0.3.0;echo unsafe"))
  assert.equal(versionTag("0.3.0"), "v0.3.0")
  assert.equal(versionTag("0.3.1"), "v0.3.1")
  assert.throws(() => versionTag("00.3.0"))
})
test("patch preparation requires a trusted completed publication of the current version", () => {
  const source = "a".repeat(40)
  const marker = `<!-- darkphish-release-source:${source} -->`
  const published = {
    tag_name: "v0.7.0", target_commitish: source, body: marker,
    draft: false, prerelease: false, published_at: "2026-09-07T12:00:00Z",
  }
  assert.equal(nextPatchVersion("0.7.0"), "0.7.1")
  assert.equal(assertPublishedVersion(source, published, "0.7.0"), published)
  for (const [tagSHA, invalid] of [
    [null, null],
    [source, { ...published, tag_name: "v0.6.0" }],
    [source, { ...published, draft: true }],
    [source, { ...published, prerelease: true }],
    [source, { ...published, published_at: null }],
    [source, { ...published, published_at: "not-a-date" }],
    [source, { ...published, target_commitish: "main" }],
    [source, { ...published, body: "missing provenance" }],
    ["b".repeat(40), published],
  ]) assert.throws(() => assertPublishedVersion(tagSHA, invalid, "0.7.0"))
})

test("release recovery never moves a tag or adopts unknown publication", () => {
  assert.equal(assertReleaseState(null, null, "abc"), "new")
  const release = { draft: true, body: "<!-- darkphish-release-source:abc -->" }
  assert.equal(assertReleaseState("abc", release, "abc"), "resume")
  assert.equal(assertReleaseState(null, { ...release, target_commitish: "abc" }, "abc"), "resume")
  assert.equal(assertReleaseState("abc", { ...release, draft: false }, "abc"), "published")
  assert.throws(() => assertReleaseState("old", release, "abc"))
  assert.throws(() => assertReleaseState("abc", { draft: true, body: "unknown" }, "abc"))
})
test("asset retries reuse only identical bytes and never replace", () => {
  assert.equal(assetDisposition(null, "abc", 123), "upload")
  assert.equal(assetDisposition({ digest: "sha256:abc", size: 123 }, "abc", 123), "reuse")
  assert.throws(() => assetDisposition({ digest: "sha256:old", size: 123 }, "abc", 123))
})
test("checksums cover each artifact exactly once", () => {
  const digest = "a".repeat(64)
  const hashes = new Map([["darkphish-one.zip", digest], ["darkphish-two.tar.gz", digest], ["SHA256SUMS", "unused"]])
  const first = `${digest}  darkphish-one.zip`
  const second = `${digest}  darkphish-two.tar.gz`
  assert.doesNotThrow(() => verifyChecksums(`${first}\n${second}\n`, hashes))
  assert.throws(() => verifyChecksums(`${first}\n${first}\n`, hashes))
  assert.throws(() => verifyChecksums(first, hashes))
  assert.throws(() => verifyChecksums(`${first}\n${"b".repeat(64)}  darkphish-two.tar.gz`, hashes))
})

test("preparation cannot replace a pending publication run", () => {
  const prepare = readFileSync(new URL("../.github/workflows/release-prepare.yml", import.meta.url), "utf8")
  const publish = readFileSync(new URL("../.github/workflows/release.yml", import.meta.url), "utf8")
  const group = (text) => text.match(/concurrency:\s+group: ([^\r\n]+)/)?.[1]
  assert.ok(group(prepare))
  assert.ok(group(publish))
  assert.notEqual(group(prepare), group(publish))
  assert.match(prepare, /cancel-in-progress: false/)
  assert.match(publish, /cancel-in-progress: false/)
  assert.match(publish, /schedule:\s+- cron:/)
  assert.match(publish, /github\.event_name != 'workflow_run'/)
})

test("protected synchronous merge pins a safe internal head without a queue or bypass", () => {
  const repo = "owner/repository"
  const pr = { number: 42, state: "open", draft: false, base: { ref: "main" }, head: { sha: "a".repeat(40), repo: { full_name: repo } } }
  for (const mergeable_state of ["clean", "blocked", "unstable"]) {
    assert.deepEqual(protectedMergeRequest(repo, { ...pr, mergeable_state }), {
      path: `repos/${repo}/pulls/42/merge`, method: "PUT", body: { sha: pr.head.sha, merge_method: "squash" },
    })
  }
  assert.throws(() => protectedMergeRequest(repo, { ...pr, draft: true }))
  assert.throws(() => protectedMergeRequest(repo, { ...pr, state: "closed" }))
  assert.throws(() => protectedMergeRequest(repo, { ...pr, base: { ref: "unprotected" } }))
  assert.throws(() => protectedMergeRequest(repo, { ...pr, head: { ...pr.head, sha: "--admin" } }))
  assert.throws(() => protectedMergeRequest(repo, { ...pr, head: { ...pr.head, repo: { full_name: "outside/fork" } } }))
  assert.throws(() => protectedMergeRequest("../repository", pr))
})
