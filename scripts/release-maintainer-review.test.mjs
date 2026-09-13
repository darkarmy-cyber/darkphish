import assert from "node:assert/strict"
import test from "node:test"
import { ReleaseMaintainerReviewError, releaseReviewBody, verifyReleaseMaintainerReview } from "./release-maintainer-review.mjs"

const repo = "darkarmy-cyber/darkphish"
const head = "a".repeat(40), base = "b".repeat(40)
const reviewer = { login: "oliverkko", id: 309485696, type: "User" }
const bot = { login: "github-actions[bot]", id: 41898282, type: "Bot" }

function fixture() {
  const pr = {
    number: 31, state: "open", draft: false, merged_at: null,
    title: "release: Darkphish 0.7.1", user: { ...bot },
    head: { ref: "release/v0.7.1", sha: head, repo: { full_name: repo } },
    base: { ref: "main", sha: base },
  }
  const review = {
    id: 10,
    node_id: "PRR_trusted",
    state: "APPROVED",
    commit_id: head,
    submitted_at: "2026-09-13T07:00:00Z",
    user: { ...reviewer },
    body: releaseReviewBody(repo, pr),
  }
  const files = [
    { filename: "VERSION", sha: "1".repeat(40) },
    { filename: "CHANGELOG.md", sha: "2".repeat(40) },
    { filename: "changes/release-fix.md", sha: "3".repeat(40) },
  ]
  const reviews = [review]
  const threads = []
  const get = async (path, options = {}) => {
    if (path.startsWith(`repos/${repo}/pulls/31/files?`)) return structuredClone(files)
    if (path.startsWith(`repos/${repo}/pulls/31/reviews?`)) return structuredClone(reviews)
    if (path === `repos/${repo}/pulls/31`) return structuredClone(pr)
    if (path === "graphql") {
      const query = options.body?.query || ""
      if (query.includes("... on PullRequestReview")) return { data: { node: {
        databaseId: review.id,
        body: review.body,
        submittedAt: review.submitted_at,
        lastEditedAt: null,
        author: { __typename: "User", databaseId: reviewer.id, login: reviewer.login },
        editor: null,
      } } }
      if (query.includes("reviewThreads")) return { data: { repository: { pullRequest: {
        headRefOid: head,
        reviewThreads: { nodes: structuredClone(threads), pageInfo: { hasNextPage: false, endCursor: null } },
      } } } }
    }
    throw new Error(`Unexpected path ${path}`)
  }
  return { pr, review, files, reviews, threads, get }
}

test("trusted maintainer approval certifies exact generated release head and base", async () => {
  const f = fixture()
  const evidence = await verifyReleaseMaintainerReview(repo, f.pr, { get: f.get })
  assert.equal(evidence.head, head)
  assert.equal(evidence.base, base)
  assert.equal(evidence.reviewer, "oliverkko")
})

test("real GitHub pull-file records without numeric ids are accepted and duplicate filename/sha identities fail", async () => {
  const f = fixture()
  assert.equal((await verifyReleaseMaintainerReview(repo, f.pr, { get: f.get })).head, head)
  f.files.push({ ...f.files[0] })
  await assert.rejects(verifyReleaseMaintainerReview(repo, f.pr, { get: f.get }), /Duplicate or invalid release review evidence identity/)
})

test("stale, forged, non-generated or non-approved release review fails closed", async () => {
  const mutations = [
    f => { f.review.commit_id = "c".repeat(40) },
    f => { f.review.state = "COMMENTED" },
    f => { f.review.user.id = 1 },
    f => { f.review.user.login = "someone-else" },
    f => { f.pr.user.id = 1 },
    f => { f.pr.user.login = "someone-else" },
    f => { f.pr.head.ref = "feature/not-release" },
    f => { f.pr.title = "release: wrong" },
    f => { f.files.push({ filename: "models/user.go", sha: "4".repeat(40) }) },
    f => { f.review.body = f.review.body.replace(head, "c".repeat(40)) },
    f => { f.review.body = f.review.body.replace(base, "c".repeat(40)) },
    f => { f.review.body = f.review.body.replace('"decision":"approved"', '"decision":"rejected"') },
    f => { f.reviews.push({ id: 11, node_id: "PRR_changes", state: "CHANGES_REQUESTED", commit_id: head, submitted_at: "2026-09-13T07:01:00Z", user: { login: "reviewer", id: 123, type: "User" }, body: "changes required" }) },
  ]
  for (const mutate of mutations) {
    const f = fixture(); mutate(f)
    await assert.rejects(verifyReleaseMaintainerReview(repo, f.pr, { get: f.get }), ReleaseMaintainerReviewError)
  }
})

test("edited trusted approval body fails closed even when REST submitted_at predates merge", async () => {
  const f = fixture()
  const originalGet = f.get
  f.pr.merged_at = "2026-09-13T07:02:00Z"
  f.get = async (path, options = {}) => {
    if (path === "graphql" && (options.body?.query || "").includes("... on PullRequestReview")) return { data: { node: {
      databaseId: f.review.id,
      body: f.review.body,
      submittedAt: f.review.submitted_at,
      lastEditedAt: "2026-09-13T07:03:00Z",
      author: { __typename: "User", databaseId: reviewer.id, login: reviewer.login },
      editor: { __typename: "User", databaseId: reviewer.id, login: reviewer.login },
    } } }
    return originalGet(path, options)
  }
  await assert.rejects(verifyReleaseMaintainerReview(repo, f.pr, { get: f.get }), /edited after submission/)
})

test("unresolved review threads block generated release approval", async () => {
  const f = fixture()
  f.threads.push({ id: "PRRT_unresolved", isResolved: false })
  await assert.rejects(verifyReleaseMaintainerReview(repo, f.pr, { get: f.get }), /unresolved review thread/)
})

test("review thread pagination stays bound to the exact head", async () => {
  const f = fixture(), originalGet = f.get
  let threadPage = 0
  f.get = async (path, options = {}) => {
    if (path === "graphql" && (options.body?.query || "").includes("reviewThreads")) {
      threadPage += 1
      if (threadPage === 1) return { data: { repository: { pullRequest: {
        headRefOid: head,
        reviewThreads: { nodes: [{ id: "PRRT_1", isResolved: true }], pageInfo: { hasNextPage: true, endCursor: "next" } },
      } } } }
      return { data: { repository: { pullRequest: {
        headRefOid: "d".repeat(40),
        reviewThreads: { nodes: [], pageInfo: { hasNextPage: false, endCursor: null } },
      } } } }
    }
    return originalGet(path, options)
  }
  await assert.rejects(verifyReleaseMaintainerReview(repo, f.pr, { get: f.get }), /head changed/)
})

test("review is bound to immutable PR snapshot and pre-merge chronology", async () => {
  const f = fixture()
  f.pr.merged_at = "2026-09-13T07:01:00Z"
  assert.equal((await verifyReleaseMaintainerReview(repo, f.pr, { get: f.get })).head, head)
  f.pr.merged_at = "2026-09-13T06:59:59Z"
  await assert.rejects(verifyReleaseMaintainerReview(repo, f.pr, { get: f.get }), /after merge/)

  const g = fixture()
  const get = g.get
  g.get = async (path, options) => path === `repos/${repo}/pulls/31`
    ? { ...structuredClone(g.pr), head: { ...g.pr.head, sha: "d".repeat(40) } }
    : get(path, options)
  await assert.rejects(verifyReleaseMaintainerReview(repo, g.pr, { get: g.get }), /changed during/)
})
