import assert from "node:assert/strict"
import test from "node:test"
import { verifyReleaseMaintainerReview } from "./release-maintainer-review.mjs"

const repo = "darkarmy-cyber/darkphish"
const head = "64241df5e4225693105e1a5989c12d453f76f8ba"
const base = "eef3789a3f7f23b03fd233716a7e0c5209a8c4c9"
const merge = "4be0bc0d1a6a2f158da5aaed996dcd87bab3bd80"
const bot = { login: "github-actions[bot]", id: 41898282, type: "Bot" }
const owner = { login: "oliverkko", id: 309485696, type: "User" }

function fixture() {
  const pr = {
    number: 119, state: "closed", draft: false, merged_at: "2026-09-27T21:50:12Z", merge_commit_sha: merge,
    merged_by: { ...owner }, title: "release: Darkphish 0.20.2", user: { ...bot },
    head: { ref: "release/v0.20.2", sha: head, repo: { full_name: repo } },
    base: { ref: "main", sha: base },
  }
  const files = [
    { filename: "CHANGELOG.md", status: "modified", sha: "245ae453bed0ef41f389cd27f26a99b97621eb8d", additions: 7, deletions: 0 },
    { filename: "VERSION", status: "modified", sha: "727d97b9bb2cf88ce2d1b361bb071b4d13f10f4e", additions: 1, deletions: 1 },
    { filename: "changes/recover-v0-20-1-review-provenance.md", status: "removed", sha: "8e179a33aab5dc1a522acfe2b4b6a98d67d54935", additions: 0, deletions: 6 },
    { filename: "changes/recover-v0-20-1-run-manifest.md", status: "removed", sha: "517fe97122ccfd49b5316e64705b6485eb89d265", additions: 0, deletions: 5 },
  ]
  const threads = []
  const finalPR = structuredClone(pr)
  const get = async (path, options = {}) => {
    if (path.startsWith(`repos/${repo}/pulls/119/files?`)) return structuredClone(files)
    if (path.startsWith(`repos/${repo}/pulls/119/reviews?`)) return []
    if (path === `repos/${repo}/pulls/119`) return structuredClone(finalPR)
    if (path === "graphql" && options.body?.query?.includes("reviewThreads")) return { data: { repository: { pullRequest: {
      headRefOid: head,
      reviewThreads: { nodes: structuredClone(threads), pageInfo: { hasNextPage: false, endCursor: null } },
    } } } }
    throw new Error(`Unexpected path ${path}`)
  }
  return { pr, files, threads, finalPR, get, verify: () => verifyReleaseMaintainerReview(repo, pr, { get }) }
}

test("only the immutable generated v0.20.2 merge receives the audited recovery authorization", async () => {
  const f = fixture(), evidence = await f.verify()
  assert.equal(evidence.auditedRelease0202, true)
  assert.equal(evidence.head, head)
  assert.equal(evidence.base, base)
  assert.equal(evidence.mergeCommit, merge)
})

test("v0.20.2 recovery fails closed on identity, tree, chronology or review-state drift", async () => {
  const changes = [
    f => { f.pr.number = 118 },
    f => { f.pr.head.ref = "release/v0.20.3"; f.pr.title = "release: Darkphish 0.20.3" },
    f => { f.pr.head.sha = "a".repeat(40) },
    f => { f.pr.base.sha = "b".repeat(40) },
    f => { f.pr.merge_commit_sha = "c".repeat(40) },
    f => { f.pr.merged_at = "2026-09-27T21:50:13Z" },
    f => { f.pr.merged_by = { ...bot } },
    f => { f.files[0].sha = "d".repeat(40) },
    f => { f.files[0].status = "added" },
    f => { f.files[0].additions = 8 },
    f => { f.files.pop() },
    f => { f.files.push({ filename: "scripts/release-publish.mjs", status: "modified", sha: "e".repeat(40), additions: 1, deletions: 1 }) },
    f => { f.threads.push({ id: "PRRT_unresolved", isResolved: false }) },
    f => { f.finalPR.merge_commit_sha = "f".repeat(40) },
    f => { f.finalPR.merged_by = { ...bot } },
  ]
  for (const change of changes) {
    const f = fixture(); change(f)
    await assert.rejects(f.verify())
  }
})
