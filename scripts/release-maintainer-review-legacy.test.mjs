import assert from "node:assert/strict"
import test from "node:test"
import { verifyReleaseMaintainerReview } from "./release-maintainer-review.mjs"

const repo = "darkarmy-cyber/darkphish", head = "a".repeat(40), base = "b".repeat(40), merge = "c".repeat(40)
const at = n => `2026-09-07T00:00:${String(n).padStart(2, "0")}Z`
function fixture() {
  const pr = { number: 20, state: "closed", draft: false, merged_at: at(20), merge_commit_sha: merge,
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
    f => { f.pr.merged_at = at(6) },
    f => { f.pr.state = "open"; f.pr.merged_at = null },
    f => { f.threads = [{ isResolved: false }] },
    f => { f.reviews = [{ id: 10, state: "CHANGES_REQUESTED", user: { login: "reviewer" } }] },
    f => { f.finalPR = { ...f.pr, merge_commit_sha: "d".repeat(40) } },
  ]) {
    const f = fixture(); mutate(f)
    await assert.rejects(f.verify())
  }
})
