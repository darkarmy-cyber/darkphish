import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"
import {
  assetDisposition, assertGeneratedCommits, assertPublishedVersion, assertReleaseState,
  checksPassed, expectedReleaseAssetNames, generatedPath, nextPatchVersion,
  peelTagToCommit, protectedMergeRequest, verifyChecksums, verifyPublishedAssetManifest, versionTag,
} from "./release-lib.mjs"

const bot = { login: "github-actions[bot]", type: "Bot", id: 41898282 }

function publishedFixture(version = "0.7.0") {
  const source = "a".repeat(40), marker = `<!-- darkphish-release-source:${source} -->`
  const assets = expectedReleaseAssetNames(version).map((name, index) => ({
    name, state: "uploaded", digest: `sha256:${String(index + 1).padStart(64, "a")}`.slice(0, 71),
    size: index + 1, uploader: bot,
    browser_download_url: `https://github.com/owner/repository/releases/download/v${version}/${name}`,
  }))
  return { source, assets, published: { tag_name: `v${version}`, target_commitish: source, body: marker, author: bot, assets, draft: false, prerelease: false, published_at: "2026-09-07T12:00:00Z" } }
}

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
  const { source, assets, published } = publishedFixture()
  assert.equal(nextPatchVersion("0.7.0"), "0.7.1")
  assert.equal(assertPublishedVersion(source, published, "0.7.0"), published)
  const invalid = [
    [null, null], [source, { ...published, author: { login: "github-actions[bot]", type: "Bot" } }],
    [source, { ...published, author: { ...bot, id: 1 } }], [source, { ...published, author: { login: "maintainer", type: "User", id: 41898282 } }],
    [source, { ...published, tag_name: "v0.6.0" }], [source, { ...published, draft: true }], [source, { ...published, prerelease: true }],
    [source, { ...published, published_at: null }], [source, { ...published, published_at: "not-a-date" }], [source, { ...published, published_at: "0" }],
    [source, { ...published, published_at: "2026-09-07" }], [source, { ...published, published_at: "09/07/2026 12:00:00" }],
    [source, { ...published, published_at: "2026-09-07T12:00:00+00:00" }], [source, { ...published, published_at: "2026-02-30T00:00:00Z" }],
    [source, { ...published, published_at: "2026-01-01T24:00:00Z" }], [source, { ...published, published_at: "2026-13-01T00:00:00Z" }],
    [source, { ...published, published_at: "2026-04-31T00:00:00Z" }], [source, { ...published, target_commitish: "main" }],
    [source, { ...published, body: "missing provenance" }], [source, { ...published, assets: assets.slice(0, 6) }],
    [source, { ...published, assets: [...assets.slice(0, 6), { ...assets[6], name: "unexpected.bin" }] }],
    [source, { ...published, assets: [...assets.slice(0, 6), assets[0]] }],
    [source, { ...published, assets: assets.map((a, i) => i ? a : { ...a, uploader: { login: "github-actions[bot]", type: "Bot" } }) }],
    [source, { ...published, assets: assets.map((a, i) => i ? a : { ...a, uploader: { ...bot, id: 1 } }) }],
    [source, { ...published, assets: assets.map((a, i) => i ? a : { ...a, digest: null }) }], ["b".repeat(40), published],
  ]
  for (const [tagSHA, value] of invalid) assert.throws(() => assertPublishedVersion(tagSHA, value, "0.7.0"))
})

test("published asset names are cryptographically bound by SHA256SUMS", async () => {
  const { assets, published } = publishedFixture()
  const payloads = assets.filter((asset) => asset.name !== "SHA256SUMS")
  const manifest = payloads.map((asset) => `${asset.digest.slice(7)}  ${asset.name}`).join("\n") + "\n"
  const download = async () => new Response(manifest, { status: 200 })
  assert.equal(await verifyPublishedAssetManifest(published, { download }), true)
  const renamed = structuredClone(published)
  const first = renamed.assets.find((asset) => asset.name !== "SHA256SUMS")
  first.name = first.name.replace("darwin", "swapped")
  await assert.rejects(verifyPublishedAssetManifest(renamed, { download }), /cryptographically bound/)
  const wrong = async () => new Response(manifest.replace(payloads[0].digest.slice(7), "f".repeat(64)), { status: 200 })
  await assert.rejects(verifyPublishedAssetManifest(published, { download: wrong }), /cryptographically bound/)
})

test("release tag peeling accepts only a bounded chain ending in a commit", async () => {
  const commit = "a".repeat(40), tag1 = "b".repeat(40), tag2 = "c".repeat(40), tree = "d".repeat(40)
  const objects = new Map([[tag1, { object: { type: "commit", sha: commit } }], [tag2, { object: { type: "tag", sha: tag1 } }]])
  const fetchTag = async sha => objects.get(sha)
  assert.equal(await peelTagToCommit({ object: { type: "commit", sha: commit } }, fetchTag), commit)
  assert.equal(await peelTagToCommit({ object: { type: "tag", sha: tag1 } }, fetchTag), commit)
  assert.equal(await peelTagToCommit({ object: { type: "tag", sha: tag2 } }, fetchTag), commit)
  await assert.rejects(peelTagToCommit({ object: { type: "tree", sha: tree } }, fetchTag))
  await assert.rejects(peelTagToCommit({ object: { type: "blob", sha: tree } }, fetchTag))
  await assert.rejects(peelTagToCommit({ object: { type: "tag", sha: "e".repeat(40) } }, fetchTag))
  await assert.rejects(peelTagToCommit({ object: { type: "tag", sha: tag1 } }, async sha => ({ object: { type: "tag", sha } })))
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
  const digest = "a".repeat(64), hashes = new Map([["darkphish-one.zip", digest], ["darkphish-two.tar.gz", digest], ["SHA256SUMS", "unused"]])
  const first = `${digest}  darkphish-one.zip`, second = `${digest}  darkphish-two.tar.gz`
  assert.doesNotThrow(() => verifyChecksums(`${first}\n${second}\n`, hashes))
  assert.throws(() => verifyChecksums(`${first}\n${first}\n`, hashes))
  assert.throws(() => verifyChecksums(first, hashes))
  assert.throws(() => verifyChecksums(`${first}\n${"b".repeat(64)}  darkphish-two.tar.gz`, hashes))
})

test("write-capable security release and merge workflows cannot be branch-dispatched", () => {
  for (const path of ["release.yml", "release-prepare.yml", "dependabot-automerge.yml", "automerge.yml", "codeql.yml"]) {
    const workflow = readFileSync(new URL(`../.github/workflows/${path}`, import.meta.url), "utf8")
    assert.doesNotMatch(workflow, /\bworkflow_dispatch\s*:/, path)
  }
  for (const path of ["release.yml", "release-prepare.yml", "dependabot-automerge.yml", "automerge.yml"]) {
    const workflow = readFileSync(new URL(`../.github/workflows/${path}`, import.meta.url), "utf8")
    assert.match(workflow, /github\.ref == 'refs\/heads\/main'/, path)
  }
  const publish = readFileSync(new URL("../.github/workflows/release.yml", import.meta.url), "utf8")
  const prepare = readFileSync(new URL("../.github/workflows/release-prepare.yml", import.meta.url), "utf8")
  assert.match(publish, /workflow_run:/); assert.match(publish, /branches: \[main\]/)
  assert.match(prepare, /workflow_run:/); assert.match(prepare, /branches: \[main\]/)
})

test("release stays draft until assets and final gates succeed", () => {
  const publish = readFileSync(new URL("./release-publish.mjs", import.meta.url), "utf8")
  assert.match(publish, /actor\?\.id === 41898282/)
  assert.match(publish, /draft: true/)
  assert.match(publish, /assertTrustedDraftRelease\(release, sha\)/)
  assert.match(publish, /asset upload failed[\s\S]*draft release retained/)
  const patch = publish.indexOf('method: "PATCH"')
  const finalMain = publish.lastIndexOf("await protectedMain(repo, sha)")
  const finalCodeQL = publish.lastIndexOf("await verifyCodeQLBaseline(repo, sha)")
  const uploaded = publish.indexOf("const uploaded =")
  assert.ok(patch > uploaded && patch > finalMain && patch > finalCodeQL)
  assert.match(publish.slice(patch), /draft: false/)
  assert.match(publish, /assertTrustedPublishedRelease\(published, sha\)/)
})

test("PR CodeQL is read-only and fails closed on local SARIF while main can upload", () => {
  const workflow = readFileSync(new URL("../.github/workflows/codeql.yml", import.meta.url), "utf8")
  const pr = workflow.match(/analyze-pr:[\s\S]*?\n  analyze-main:/)?.[0] || ""
  assert.match(pr, /security-events: read/)
  assert.match(pr, /upload: never/)
  assert.match(pr, /CodeQL produced no SARIF output/)
  assert.match(pr, /CodeQL found \$\{findings\} local result/)
  assert.doesNotMatch(pr, /security-events: write/)
  assert.match(workflow, /analyze-main:[\s\S]*security-events: write/)
})

test("protected engineering Dependabot merge and publication share one non-cancelling serialization group", () => {
  const merge = readFileSync(new URL("../.github/workflows/automerge.yml", import.meta.url), "utf8")
  const dependabot = readFileSync(new URL("../.github/workflows/dependabot-automerge.yml", import.meta.url), "utf8")
  const publish = readFileSync(new URL("../.github/workflows/release.yml", import.meta.url), "utf8")
  const group = text => text.match(/concurrency:\s+group: ([^\r\n]+)/)?.[1]
  assert.equal(group(merge), "protected-main-mutation")
  assert.equal(group(dependabot), "protected-main-mutation")
  assert.equal(group(publish), "protected-main-mutation")
  assert.match(merge, /cancel-in-progress: false/)
  assert.match(dependabot, /cancel-in-progress: false/)
  assert.doesNotMatch(dependabot, /--auto/)
  assert.match(dependabot, /--match-head-commit/)
  assert.match(publish, /cancel-in-progress: false/)
})

test("pending publication freezes ordinary and dependency main mutation paths", () => {
  const lib = readFileSync(new URL("./release-lib.mjs", import.meta.url), "utf8")
  const dependency = readFileSync(new URL("./require-green-main.mjs", import.meta.url), "utf8")
  const prepare = readFileSync(new URL("./release-prepare.mjs", import.meta.url), "utf8")
  assert.match(lib, /current VERSION is not fully published; protected main is frozen/)
  assert.match(lib, /release state changed before merge; protected main remains frozen/)
  assert.match(dependency, /await assertCurrentVersionPublished\(repo\)/)
  assert.match(prepare, /releaseMerge: true/)
})

test("preparation cannot replace a pending publication run", () => {
  const prepare = readFileSync(new URL("../.github/workflows/release-prepare.yml", import.meta.url), "utf8")
  const publish = readFileSync(new URL("../.github/workflows/release.yml", import.meta.url), "utf8")
  const group = text => text.match(/concurrency:\s+group: ([^\r\n]+)/)?.[1]
  assert.ok(group(prepare)); assert.ok(group(publish)); assert.notEqual(group(prepare), group(publish))
  assert.match(prepare, /cancel-in-progress: false/); assert.match(publish, /cancel-in-progress: false/)
  assert.match(publish, /schedule:\s+- cron:/); assert.match(publish, /github\.event_name != 'workflow_run'/)
})

test("protected synchronous merge pins a safe internal head without a queue or bypass", () => {
  const repo = "owner/repository", pr = { number: 42, state: "open", draft: false, base: { ref: "main" }, head: { sha: "a".repeat(40), repo: { full_name: repo } } }
  for (const mergeable_state of ["clean", "blocked", "unstable"]) assert.deepEqual(protectedMergeRequest(repo, { ...pr, mergeable_state }), { path: `repos/${repo}/pulls/42/merge`, method: "PUT", body: { sha: pr.head.sha, merge_method: "squash" } })
  assert.throws(() => protectedMergeRequest(repo, { ...pr, draft: true }))
  assert.throws(() => protectedMergeRequest(repo, { ...pr, state: "closed" }))
  assert.throws(() => protectedMergeRequest(repo, { ...pr, base: { ref: "unprotected" } }))
  assert.throws(() => protectedMergeRequest(repo, { ...pr, head: { ...pr.head, sha: "--admin" } }))
  assert.throws(() => protectedMergeRequest(repo, { ...pr, head: { ...pr.head, repo: { full_name: "outside/fork" } } }))
  assert.throws(() => protectedMergeRequest("../repository", pr))
})
