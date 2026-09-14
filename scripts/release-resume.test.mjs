import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"
import { resumeRelease } from "./release-resume.mjs"

const main = "e".repeat(40), bot = { login: "github-actions[bot]", id: 41898282, type: "Bot" }
function fixture() {
  const f = { release: { id: 388329244, tag_name: "v0.7.1", target_commitish: "73bf5948ed19cd918638453d478ef3f1cbe83d89",
    name: "Darkphish 0.7", draft: true, prerelease: false, author: bot, published_at: "2026-09-14T11:01:32Z", body: "canonical" },
  pr: { number: 58, state: "closed", merged_at: "2026-09-14T14:00:00Z", merge_commit_sha: main,
    base: { ref: "main" }, head: { repo: { full_name: "darkarmy-cyber/darkphish" }, ref: "codex/threads/019fb3b4-63f2-7180-8a29-babee7e6a51b/release071-finalize" } },
  writes: [], verifies: 0, reviews: 0, executions: 0 }
  f.assets = Array.from({ length: 8 }, (_, i) => ({ id: i + 1, name: i ? `asset-${i}` : "SHA256SUMS", size: 1,
    digest: i ? `sha256:${"a".repeat(64)}` : "sha256:d41dea173bb71a7230b26bd10b60209285a9e9892a07e7267dd8e46c63a8e7b7", state: "uploaded", uploader: bot }))
  f.request = async (path, options = {}) => {
    if (options.method) {
      assert.equal(options.method, "PATCH")
      assert.equal(path, "repos/darkarmy-cyber/darkphish/releases/388329244")
      f.writes.push(options.body); f.release.draft = options.body.draft
      if (!options.body.draft && f.changePublicationTime) f.release.published_at = "2026-09-14T15:00:00Z"
      return structuredClone(f.release)
    }
    if (path.endsWith("/pulls/58")) return structuredClone(f.pr)
    if (path.includes("/assets?")) return structuredClone(f.changedAssets || f.assets)
    if (path.includes("/releases/")) return structuredClone(f.release)
    throw new Error(`Unexpected request ${path}`)
  }
  f.deps = { request: f.request, hold: () => f.hold || null, log: () => {},
    execution: async () => { f.executions++; if (f.badMain) throw new Error("Main changed") },
    reviews: async () => { f.reviews++; if (f.badReviews) throw new Error("Missing review") },
    verify: async () => { f.verifies++; if (f.badProvenance || (f.failPost && f.verifies > 1)) throw new Error("Invalid provenance")
      return { body: "canonical", assets: new Map(f.assets.map(a => [a.name, a])) } } }
  f.run = () => resumeRelease(main, f.deps)
  return f
}

test("resumption publishes only the verified exact draft and checks again afterwards", async () => {
  const f = fixture(); await f.run()
  assert.deepEqual(f.writes, [{ draft: false, prerelease: false, make_latest: "true" }])
  assert.equal(f.verifies, 2); assert.equal(f.reviews, 3); assert.equal(f.executions, 3)
})

test("already verified public release is never republished", async () => {
  const f = fixture(); f.release.draft = false; await f.run(); assert.equal(f.writes.length, 0)
})

test("hold, wrong main/PR, missing review, provenance and changed identity prevent publication", async () => {
  for (const change of [f => { f.hold = {} }, f => { f.badMain = true }, f => { f.badReviews = true },
    f => { f.badProvenance = true }, f => { f.pr.merge_commit_sha = "b".repeat(40) },
    f => { f.pr.state = "open" }, f => { f.release.id = 388031151 },
    f => { f.release.target_commitish = "b".repeat(40) }, f => { f.release.author = { ...bot, id: 1 } },
    f => { f.release.body = "changed" }, f => { f.assets[0].digest = "sha256:" + "b".repeat(64) },
    f => { f.changedAssets = f.assets.slice(1) }, f => { f.release.published_at = null }]) {
    const f = fixture(); change(f); await assert.rejects(f.run()); assert.equal(f.writes.length, 0)
  }
})

test("post-publication failures withdraw the same release without replacing or deleting assets", async () => {
  for (const change of [f => { f.failPost = true }, f => { f.changePublicationTime = true }]) {
    const f = fixture(); change(f); await assert.rejects(f.run())
    assert.deepEqual(f.writes, [{ draft: false, prerelease: false, make_latest: "true" }, { draft: true, prerelease: false, make_latest: "false" }])
    assert.equal(f.release.draft, true)
  }
})

test("resumption workflow runs immutable main code in the shared serialization group", () => {
  const workflow = readFileSync(new URL("../.github/workflows/release-resume.yml", import.meta.url), "utf8")
  assert.match(workflow, /workflow_run:/)
  assert.doesNotMatch(workflow, /workflow_dispatch:|pull_request_target:/)
  assert.match(workflow, /ref: \$\{\{ github\.event\.workflow_run\.head_sha \}\}/)
  assert.match(workflow, /group: protected-main-mutation\n\s+cancel-in-progress: false/)
  assert.match(workflow, /persist-credentials: false/)
})
