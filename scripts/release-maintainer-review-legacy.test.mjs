import assert from "node:assert/strict"
import test from "node:test"
import { verifyReleaseMaintainerReview } from "./release-maintainer-review.mjs"

const repo = "darkarmy-cyber/darkphish", head = "67848d4dcd25023899fa587e5d6b2dbd6f78b55b",
  base = "38504372fb5f59ce989e15228fef70f0709f5e48", merge = "355d2881a4d5eea162d46a1c155998b1fb8c9f36"
const at = n => `2026-09-07T00:00:${String(n).padStart(2, "0")}Z`
function fixture() {
  const pr = { number: 20, state: "closed", draft: false, merged_at: "2026-09-07T08:29:08Z", merge_commit_sha: merge,
    merged_by: { login: "github-actions[bot]", id: 41898282, type: "Bot" },
    title: "release: Darkphish 0.7.0", user: { login: "github-actions[bot]", id: 41898282, type: "Bot" },
    head: { ref: "release/v0.7.0", sha: head, repo: { full_name: repo } }, base: { ref: "main", sha: base } }
  const actor = { login: "chatgpt-codex-connector[bot]", id: 199175422, type: "Bot" }
  const comment = (id, body, created, updated = created) => ({ id, node_id: `node-${id}`, body,
    created_at: at(created), updated_at: at(updated), user: actor, performed_via_github_app: { id: 1144995, slug: "chatgpt-codex-connector" } })
  const marker = JSON.stringify({ repository: repo, pullRequestNumber: 20, headSha: head, status: "completed" })
  const row = (kind, n) => `| **${kind} Review** | ✅ **Completed** <relative-time datetime="${at(n)}">${at(n)}</relative-time> | \`${head.slice(0, 7)}\` | Manual request |`
  const f = { pr, reviews: [], threads: [], comments: [
    comment(1, `<!-- codex-pull-request-review-summary -->\n<!-- codex-security-review:v1 ${marker} -->\n${row("Code", 5)}\n${row("Security", 9)}`, 0, 10),
    comment(2, `Codex Review: Didn't find any major issues.\n\n**Reviewed commit:** \`${head}\``, 4),
    comment(3, `Security review completed. No security issues were found in this pull request.\n\n**Reviewed commit:** \`${head}\``, 8),
  ] }
  f.get = async (path, options = {}) => {
    if (path.includes("/files?")) return [{ filename: "VERSION", sha: "1".repeat(40) }]
    if (path.includes("/reviews?")) return structuredClone(f.reviews)
    if (path.includes("/issues/20/comments?")) return structuredClone(f.comments)
    if (path.includes("/pulls/20/comments?")) return []
    if (path === `repos/${repo}/pulls/20`) return structuredClone(f.finalPR || pr)
    if (path === "graphql") {
      if (options.body.query.includes("nodes(ids:")) return { data: { nodes: f.comments.map(c => ({ databaseId: c.id, body: c.body,
        updatedAt: c.updated_at, lastEditedAt: c.id === 1 ? c.updated_at : null,
        author: { __typename: "Bot", databaseId: 199175422 },
        editor: c.id === 1 ? { __typename: "Bot", databaseId: 199175422 } : null })) } }
      return { data: { repository: { pullRequest: { headRefOid: head,
        reviewThreads: { nodes: f.threads, pageInfo: { hasNextPage: false, endCursor: null } } } } } }
    }
    throw new Error(`Unexpected path ${path}`)
  }
  f.verify = () => verifyReleaseMaintainerReview(repo, pr, { get: f.get })
  return f
}

test("historical releases require both authentic exact-head reviews before merge", async () => {
  const f = fixture(), result = await f.verify()
  assert.equal(result.historicalConnectorReviews, true)
  assert.equal(result.mergeCommit, merge)
  assert.equal(result.head, head)
})

test("absent, incomplete, forged, late or unresolved historical reviews fail closed", async () => {
  for (const mutate of [
    f => { f.comments = [] },
    f => { f.comments.pop() },
    f => { f.comments[2].user = { login: "impostor", id: 1, type: "User" } },
    f => { f.comments[2].created_at = "2026-09-07T09:00:00Z"; f.comments[2].updated_at = f.comments[2].created_at },
    f => { f.pr.state = "open"; f.pr.merged_at = null },
    f => { f.threads = [{ isResolved: false }] },
    f => { f.reviews = [{ id: 10, state: "CHANGES_REQUESTED", user: { login: "reviewer" } }] },
    f => { f.finalPR = { ...f.pr, merge_commit_sha: "d".repeat(40) } },
  ]) {
    const f = fixture(); mutate(f)
    await assert.rejects(f.verify())
  }
})

test("only the audited pre-attestation protected merge may use historical reviews", async () => {
  for (const mutate of [
    f => { f.pr.number = 21 },
    f => { f.pr.head.sha = "a".repeat(40) },
    f => { f.pr.base.sha = "b".repeat(40) },
    f => { f.pr.merge_commit_sha = "c".repeat(40) },
    f => { f.pr.merged_at = "2026-09-14T00:00:00Z" },
    f => { f.pr.merged_by = { login: "oliverkko", id: 309485696, type: "User" } },
    f => { f.pr.merged_by = undefined },
    f => { f.pr.head.ref = "release/v0.7.1"; f.pr.title = "release: Darkphish 0.7.1" },
  ]) {
    const f = fixture(); mutate(f)
    await assert.rejects(f.verify(), /not the audited pre-attestation protected merge/)
  }
})
