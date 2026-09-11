import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"
import { assetDisposition, assertGeneratedCommits, assertPublishedVersion, assertReleaseState, checksPassed, generatedPath, nextPatchVersion, peelTagToCommit, protectedMergeRequest, verifyChecksums, versionTag } from "./release-lib.mjs"

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
test("patch preparation requires trusted publisher and exact completed artifact set", () => {
  const source = "a".repeat(40)
  const marker = `<!-- darkphish-release-source:${source} -->`
  const bot = { login: "github-actions[bot]", type: "Bot", id: 41898282 }
  const digest = `sha256:${"b".repeat(64)}`
  const names = [
    "darkphish-v0.7.0-darwin-amd64.tar.gz",
    "darkphish-v0.7.0-darwin-arm64.tar.gz",
    "darkphish-v0.7.0-linux-amd64.tar.gz",
    "darkphish-v0.7.0-linux-arm64.tar.gz",
    "darkphish-v0.7.0-windows-amd64.zip",
    "darkphish-v0.7.0.spdx.json",
    "SHA256SUMS",
  ]
  const assets = names.map((name, index) => ({ name, state: "uploaded", digest, size: index + 1, uploader: bot }))
  const published = {
    tag_name: "v0.7.0", target_commitish: source, body: marker, author: bot, assets,
    draft: false, prerelease: false, published_at: "2026-09-07T12:00:00Z",
  }
  assert.equal(nextPatchVersion("0.7.0"), "0.7.1")
  assert.equal(assertPublishedVersion(source, published, "0.7.0"), published)
  for (const [tagSHA, invalid] of [
    [null, null],
    [source, { ...published, author: { login: "github-actions[bot]", type: "Bot" } }],
    [source, { ...published, author: { ...bot, id: 1 } }],
    [source, { ...published, author: { login: "maintainer", type: "User", id: 41898282 } }],
    [source, { ...published, tag_name: "v0.6.0" }],
    [source, { ...published, draft: true }],
    [source, { ...published, prerelease: true }],
    [source, { ...published, published_at: null }],
    [source, { ...published, published_at: "not-a-date" }],
    [source, { ...published, published_at: "0" }],
    [source, { ...published, published_at: "2026-09-07" }],
    [source, { ...published, published_at: "09/07/2026 12:00:00" }],
    [source, { ...published, published_at: "2026-09-07T12:00:00+00:00" }],
    [source, { ...published, target_commitish: "main" }],
    [source, { ...published, body: "missing provenance" }],
    [source, { ...published, assets: assets.slice(0, 6) }],
    [source, { ...published, assets: [...assets.slice(0, 6), { ...assets[6], name: "unexpected.bin" }] }],
    [source, { ...published, assets: [...assets.slice(0, 6), assets[0]] }],
    [source, { ...published, assets: assets.map((asset, index) => index ? asset : { ...asset, uploader: { login: "github-actions[bot]", type: "Bot" } }) }],
    [source, { ...published, assets: assets.map((asset, index) => index ? asset : { ...asset, uploader: { ...bot, id: 1 } }) }],
    [source, { ...published, assets: assets.map((asset, index) => index ? asset : { ...asset, digest: null }) }],
    ["b".repeat(40), published],
  ]) assert.throws(() => assertPublishedVersion(tagSHA, invalid, "0.7.0"))
})

test("release tag peeling accepts only a bounded chain ending in a commit", async () => {
  const commit = "a".repeat(40), tag1 = "b".repeat(40), tag2 = "c".repeat(40), tree = "d".repeat(40)
  const objects = new Map([
    [tag1, { object: { type: "commit", sha: commit } }],
    [tag2, { object: { type: "tag", sha: tag1 } }],
  ])
  const fetchTag = async sha => objects.get(sha)
  assert.equal(await peelTagToCommit({ object: { type: "commit", sha: commit } }, fetchTag), commit)
  assert.equal(await peelTagToCommit({ object: { type: "tag", sha: tag1 } }, fetchTag), commit)
  assert.equal(await peelTagToCommit({ object: { type: "tag", sha: tag2 } }, fetchTag), commit)
  await assert.rejects(peelTagToCommit({ object: { type: "tree", sha: tree } }, fetchTag))
  await assert.rejects(peelTagToCommit({ object: { type: "blob", sha: tree } }, fetchTag))
  await assert.rejects(peelTagToCommit({ object: { type: "tag", sha: "e".repeat(40) } }, fetchTag))
  const cyclic = async sha => ({ object: { type: "tag", sha } })
  await assert.rejects(peelTagToCommit({ object: { type: "tag", sha: tag1 } }, cyclic))
  const deep = new Map()
  for (let i = 0; i < 10; i++) deep.set(String(i).padStart(40, "0"), { object: { type: "tag", sha: String(i + 1).padStart(40, "0") } })
  await assert.rejects(peelTagToCommit({ object: { type: "tag", sha: String(0).padStart(40, "0") } }, async sha => deep.get(sha), 2))
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
