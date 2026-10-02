import assert from "node:assert/strict"
import test from "node:test"
import { releaseReviewBody, verifyReleaseMaintainerReview } from "./release-maintainer-review.mjs"

const repo = "darkarmy-cyber/darkphish"
const head = "5f8b6de5a6ed4076b987747c32d615b5b9bd180a"
const base = "443d247b1636ea8e46c7339378174d3c88a0abf5"
const merge = "86cb7173c553d68bc3afac204948aa923a32bc83"
const bot = { login: "github-actions[bot]", id: 41898282, type: "Bot" }
const owner = { login: "oliverkko", id: 309485696, type: "User" }

function fixture() {
  const pr = {
    number: 181, state: "closed", draft: false, merged_at: "2026-10-02T12:24:22Z",
    merge_commit_sha: merge, merged_by: { ...owner }, title: "release: Darkphish 0.23.1", user: { ...bot },
    head: { ref: "release/v0.23.1", sha: head, repo: { full_name: repo } },
    base: { ref: "main", sha: base },
  }
  const files = [
    { filename: "CHANGELOG.md", status: "modified", sha: "fc349b2e5757db39b9d4f0488499195463110448", additions: 6, deletions: 0 },
    { filename: "VERSION", status: "modified", sha: "610e28725be0c15d7ab0ced6fa2b394233a49f38", additions: 1, deletions: 1 },
    { filename: "changes/fix-retired-training-update-bridge.md", status: "removed", sha: "d85e275e9bdc197211b02d7189813d4f274d5a35", additions: 0, deletions: 5 },
  ]
  const reviews = [{
    id: 5392013646, node_id: "PRR_kwDOUPnYXM8AAAABQAVWjg", state: "APPROVED", commit_id: head,
    submitted_at: "2026-10-02T12:54:20Z", user: { ...owner }, body: releaseReviewBody(repo, pr),
  }]
  const threads = []
  const finalPR = structuredClone(pr)
  const get = async (path, options = {}) => {
    if (path.startsWith(`repos/${repo}/pulls/181/files?`)) return structuredClone(files)
    if (path.startsWith(`repos/${repo}/pulls/181/reviews?`)) return structuredClone(reviews)
    if (path === `repos/${repo}/pulls/181`) return structuredClone(finalPR)
    if (path === "graphql" && options.body?.query?.includes("PullRequestReview")) {
      const review = reviews.find(row => row.node_id === options.body.variables.id)
      return { data: { node: review && {
        databaseId: review.id, body: review.body, submittedAt: review.submitted_at, lastEditedAt: null,
        author: { __typename: "User", databaseId: owner.id, login: owner.login }, editor: null,
      } } }
    }
    if (path === "graphql" && options.body?.query?.includes("reviewThreads")) return { data: { repository: { pullRequest: {
      headRefOid: head,
      reviewThreads: { nodes: structuredClone(threads), pageInfo: { hasNextPage: false, endCursor: null } },
    } } } }
    throw new Error(`Unexpected path ${path}`)
  }
  return { pr, files, reviews, threads, finalPR, get, verify: () => verifyReleaseMaintainerReview(repo, pr, { get }) }
}

test("only the immutable reviewed v0.23.1 merge receives post-merge recovery authorization", async () => {
  const f = fixture()
  const evidence = await f.verify()
  assert.equal(evidence.auditedRelease0231, true)
  assert.equal(evidence.head, head)
  assert.equal(evidence.base, base)
  assert.equal(evidence.mergeCommit, merge)
  assert.equal(evidence.reviewID, 5392013646)
})

test("v0.23.1 recovery fails closed on identity, manifest, review or chronology drift", async () => {
  const changes = [
    f => { f.pr.number = 182 },
    f => { f.pr.head.sha = "a".repeat(40) },
    f => { f.pr.base.sha = "b".repeat(40) },
    f => { f.pr.merge_commit_sha = "c".repeat(40) },
    f => { f.pr.merged_at = "2026-10-02T12:24:23Z" },
    f => { f.files[0].sha = "d".repeat(40) },
    f => { f.files.pop() },
    f => { f.reviews[0].submitted_at = "2026-10-02T12:24:21Z" },
    f => { f.reviews[0].body = "forged attestation" },
    f => { f.reviews[0].body += "\nappended text" },
    f => { f.threads.push({ id: "PRRT_unresolved", isResolved: false }) },
    f => { f.finalPR.merge_commit_sha = "e".repeat(40) },
  ]
  for (const change of changes) {
    const f = fixture()
    change(f)
    await assert.rejects(f.verify())
  }
})
