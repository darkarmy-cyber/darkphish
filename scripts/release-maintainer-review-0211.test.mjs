import assert from "node:assert/strict"
import test from "node:test"
import { releaseReviewBody, verifyReleaseMaintainerReview } from "./release-maintainer-review.mjs"

const repo = "darkarmy-cyber/darkphish"
const head = "e5a4f4713cbe25d05d641c8b22e8e83da3ea69a7"
const base = "e9512fc4c564dabdc098b3bf44d3748c355d4d15"
const merge = "51f3c3b56df05d8a6fd165d227cb78d9e46ae821"
const bot = { login: "github-actions[bot]", id: 41898282, type: "Bot" }
const owner = { login: "oliverkko", id: 309485696, type: "User" }

function fixture() {
  const pr = {
    number: 128, state: "closed", draft: false, merged_at: "2026-09-29T10:41:54Z",
    merge_commit_sha: merge, merged_by: { ...owner }, title: "release: Darkphish 0.21.1", user: { ...bot },
    head: { ref: "release/v0.21.1", sha: head, repo: { full_name: repo } },
    base: { ref: "main", sha: base },
  }
  const files = [
    { filename: "CHANGELOG.md", status: "modified", sha: "45a023639a8c1dab50ef70f1f96ddd338d28eb3c", additions: 6, deletions: 0 },
    { filename: "VERSION", status: "modified", sha: "a67cebaf7ff61ccbf2741283f4341147d2fadae8", additions: 1, deletions: 1 },
    { filename: "changes/recover-withdrawn-release-draft.md", status: "removed", sha: "a8ffb0af986fe3dfad2c1f82018d2ca33d6ef39a", additions: 0, deletions: 5 },
  ]
  const reviews = [
    {
      id: 5351212983, node_id: "PRR_kwDOUPnYXM8AAAABPvUHtw", state: "APPROVED", commit_id: head,
      submitted_at: "2026-09-29T10:37:44Z", user: { ...owner },
      body: "Verified generated-only v0.21.1 release scope: VERSION bump, canonical changelog aggregation, and consumed-fragment removal. No runtime or workflow changes.",
    },
    {
      id: 5351353878, node_id: "PRR_kwDOUPnYXM8AAAABPvcuFg", state: "APPROVED", commit_id: head,
      submitted_at: "2026-09-29T10:48:56Z", user: { ...owner }, body: releaseReviewBody(repo, pr),
    },
  ]
  const threads = []
  const finalPR = structuredClone(pr)
  const get = async (path, options = {}) => {
    if (path.startsWith(`repos/${repo}/pulls/128/files?`)) return structuredClone(files)
    if (path.startsWith(`repos/${repo}/pulls/128/reviews?`)) return structuredClone(reviews)
    if (path === `repos/${repo}/pulls/128`) return structuredClone(finalPR)
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

test("only the immutable reviewed v0.21.1 merge receives recovery authorization", async () => {
  const f = fixture()
  const evidence = await f.verify()
  assert.equal(evidence.auditedRelease0211, true)
  assert.equal(evidence.head, head)
  assert.equal(evidence.base, base)
  assert.equal(evidence.mergeCommit, merge)
})

test("v0.21.1 recovery fails closed on identity, manifest, review or chronology drift", async () => {
  const changes = [
    f => { f.pr.number = 129 },
    f => { f.pr.head.sha = "a".repeat(40) },
    f => { f.pr.base.sha = "b".repeat(40) },
    f => { f.pr.merge_commit_sha = "c".repeat(40) },
    f => { f.pr.merged_at = "2026-09-29T10:41:55Z" },
    f => { f.pr.merged_by = { ...bot } },
    f => { f.files[0].sha = "d".repeat(40) },
    f => { f.files.pop() },
    f => { f.reviews[0].submitted_at = "2026-09-29T10:41:55Z" },
    f => { f.reviews[0].body = "generic approval" },
    f => { f.reviews[1].submitted_at = "2026-09-29T10:41:53Z" },
    f => { f.reviews[1].body = "forged attestation" },
    f => { f.threads.push({ id: "PRRT_unresolved", isResolved: false }) },
    f => { f.finalPR.merge_commit_sha = "e".repeat(40) },
  ]
  for (const change of changes) {
    const f = fixture()
    change(f)
    await assert.rejects(f.verify())
  }
})
