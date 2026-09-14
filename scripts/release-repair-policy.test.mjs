import assert from "node:assert/strict"
import test from "node:test"
import { verifyReleaseRepair } from "./release-repair-policy.mjs"
import { mergeReviewedPullRequest, requiredChecks } from "./release-lib.mjs"
import { ReviewGateError } from "./review-gate.mjs"

const repo = "darkarmy-cyber/darkphish", head = "a".repeat(40), base = "a2851cd96e327175b3d45b8949ad64492b52b21f"
function fixture() {
  const f = { pr: { number: 56, state: "open", draft: false, author_association: "OWNER",
    head: { sha: head, ref: "fix/historical-release-review-provenance", repo: { full_name: repo } },
    base: { ref: "main", sha: base }, labels: [{ name: "codex-automerge" }], mergeable: true, mergeable_state: "clean" },
  files: [{ filename: "scripts/release-maintainer-review.mjs", status: "modified" }], version: "0.7.1\n",
  hold: { schema: "darkphish-release-normalization-hold/v1", version: "0.7.1", source_sha: "73bf5948ed19cd918638453d478ef3f1cbe83d89" },
  checks: requiredChecks.map((name, id) => ({ id, name, status: "completed", conclusion: "success", app: { slug: "github-actions" } })),
  alerts: [], writes: [], reviews: 0, fileReads: 0 }
  f.request = async (path, options = {}) => {
    if (options.method === "PUT") { f.writes.push(options.body); return { merged: true, sha: "c".repeat(40) } }
    if (path.endsWith("/files?per_page=100&page=1")) { f.fileReads++; return f.fileReads > 1 && f.finalFiles ? f.finalFiles : f.files }
    if (path.includes("/contents/")) return { type: "file", encoding: "base64", content: Buffer.from(path.includes("/VERSION?") ? f.version : JSON.stringify(f.hold)).toString("base64") }
    if (path === `repos/${repo}`) return { full_name: repo, default_branch: "main", private: false, fork: false, allow_auto_merge: true }
    if (path.endsWith("/branches/main")) return { protected: true, commit: { sha: base } }
    if (path.endsWith("/pulls/56")) return structuredClone(f.pr)
    if (path.includes("/check-runs?")) return { check_runs: f.checks }
    if (path.includes("/code-scanning/alerts?")) return f.alerts
    throw new Error(`Unexpected request ${path}`)
  }
  f.merge = () => mergeReviewedPullRequest(repo, structuredClone(f.pr), { releaseRepair: true, request: f.request,
    verifyReviews: async () => { f.reviews++; if (f.badReview) throw new ReviewGateError("Missing security review") }, log: () => {} })
  return f
}

test("explicit repair retains merge gates and revalidates scope immediately before exact-head merge", async () => {
  const f = fixture()
  assert.equal(await f.merge(), true)
  assert.equal(f.reviews, 1)
  assert.equal(f.fileReads, 2)
  assert.deepEqual(f.writes, [{ sha: head, merge_method: "squash" }])
})

test("repair never authorizes failed checks, alerts, absent reviews or unready PR", async () => {
  for (const change of [f => { f.checks.pop() }, f => { f.alerts = [{}] }, f => { f.badReview = true },
    f => { f.pr.labels = [] }, f => { f.pr.mergeable = false }]) {
    const f = fixture(); change(f)
    assert.equal(await f.merge(), false)
    assert.equal(f.writes.length, 0)
  }
})

test("wrong PR, base, branch, files, operations, VERSION or hold cannot use repair mode", async () => {
  for (const change of [f => { f.pr.number = 57 }, f => { f.pr.base.sha = head },
    f => { f.pr.head.ref = "other" }, f => { f.pr.head.repo.full_name = "other/repo" },
    f => { f.files = [{ filename: "VERSION", status: "modified" }] },
    f => { f.files[0].status = "removed" }, f => { f.files[0].previous_filename = "other" },
    f => { f.files.push(f.files[0]) }, f => { f.files = [] }, f => { f.version = "0.8.0" },
    f => { f.hold.source_sha = head }, f => { f.hold.extra = true }]) {
    const f = fixture(); change(f)
    await assert.rejects(verifyReleaseRepair(repo, f.pr, f.request))
    assert.equal(f.writes.length, 0)
  }
})

test("scope change during reviews prevents final merge", async () => {
  const f = fixture(); f.finalFiles = [{ filename: "VERSION", status: "modified" }]
  await assert.rejects(f.merge(), /unauthorized/)
  assert.equal(f.writes.length, 0)
})
