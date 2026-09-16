import assert from "node:assert/strict"
import test from "node:test"
import { ReleaseMaintainerReviewError, releaseReviewBody, verifyReleaseMaintainerReview } from "./release-maintainer-review.mjs"
import { mergeReviewedPullRequest, requiredChecks } from "./release-lib.mjs"
import { ReviewGateError } from "./review-gate.mjs"

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

function mergeFixture() {
  const f = fixture(), original = f.get
  Object.assign(f.pr, { labels: [{ name: "codex-automerge" }], mergeable: true, mergeable_state: "clean", auto_merge: null })
  Object.assign(f, { writes: [], reviewReads: 0, connectorReads: 0,
    checks: requiredChecks.map((name, id) => ({ id, name, status: "completed", conclusion: "success", app: { slug: "github-actions" } })), alerts: [] })
  f.request = async (path, options = {}) => {
    if (options.method === "PUT") {
      assert.equal(path, `repos/${repo}/pulls/31/merge`)
      f.writes.push(options.body)
      return { merged: true, sha: "c".repeat(40) }
    }
    if (path === `repos/${repo}`) return { full_name: repo, default_branch: "main", private: false, fork: false, allow_auto_merge: true }
    if (path === `repos/${repo}/branches/main`) return { protected: true, commit: { sha: base } }
    if (path.includes("/check-runs?")) return { check_runs: f.checks }
    if (path.includes("/code-scanning/alerts?")) return f.alerts
    if (path.includes("/reviews?")) {
      f.reviewReads++
      if (f.reviewReads === 2 && f.revoke) f.review.state = "DISMISSED"
    }
    return original(path, options)
  }
  f.merge = (options = {}) => mergeReviewedPullRequest(repo, structuredClone(f.pr), {
    releaseMerge: true, request: f.request, log: () => {},
    verifyReviews: async () => { f.connectorReads++; if (f.badConnector) throw new ReviewGateError("Missing clean security review") },
    ...options,
  })
  return f
}

test("release merge authenticates maintainer attestation twice before exact-head PUT", async () => {
  const f = mergeFixture()
  assert.equal(await f.merge(), true)
  assert.equal(f.connectorReads, 1)
  assert.equal(f.reviewReads, 2)
  assert.deepEqual(f.writes, [{ sha: head, merge_method: "squash" }])
})

test("release merge cannot strand publication behind absent stale forged or revoked attestation", async () => {
  for (const mutate of [
    f => { f.reviews.length = 0 },
    f => { f.review.commit_id = "d".repeat(40) },
    f => { f.review.body = "Approved without the exact generated-release marker" },
    f => { f.review.user.id = 1 },
    f => { f.review.state = "CHANGES_REQUESTED" },
    f => { f.pr.user.id = 1 },
    f => { f.files.push({ filename: "models/user.go", sha: "e".repeat(40) }) },
    f => { f.threads.push({ id: "unresolved", isResolved: false }) },
    f => { f.revoke = true },
  ]) {
    const f = mergeFixture(); mutate(f)
    assert.equal(await f.merge(), false)
    assert.deepEqual(f.writes, [])
  }
})

test("release merge still requires connector reviews checks baseline and ready state", async () => {
  for (const mutate of [f => { f.badConnector = true }, f => { f.checks.pop() },
    f => { f.alerts.push({}) }, f => { f.pr.draft = true }, f => { f.pr.labels = [] },
    f => { f.pr.mergeable_state = "blocked" }]) {
    const f = mergeFixture(); mutate(f)
    assert.equal(await f.merge(), false)
    assert.deepEqual(f.writes, [])
  }
})

test("release review transport failure never causes a merge", async () => {
  const f = mergeFixture(), original = f.request
  await assert.rejects(f.merge({ request: async (path, options) => {
    if (path.includes("/reviews?")) throw new Error("review service unavailable")
    return original(path, options)
  } }), /review service unavailable/)
  assert.deepEqual(f.writes, [])
})

test("invalid release modes cannot bypass the publication boundary", async () => {
  for (const options of [{ releaseMerge: "true" }, { releaseRepair: "true" }, { releaseMerge: true, releaseRepair: true }]) {
    const f = mergeFixture()
    await assert.rejects(f.merge(options), /Invalid release merge mode/)
    assert.deepEqual(f.writes, [])
  }
})

test("ordinary merge does not demand generated-release-only attestation", async () => {
  const f = mergeFixture(); f.reviews.length = 0
  assert.equal(await f.merge({ releaseMerge: false }), true)
  assert.equal(f.reviewReads, 0)
})

test("trusted maintainer approval certifies exact generated release head and base", async () => {
  const f = fixture()
  const evidence = await verifyReleaseMaintainerReview(repo, f.pr, { get: f.get })
  assert.equal(evidence.head, head)
  assert.equal(evidence.base, base)
  assert.equal(evidence.reviewer, "oliverkko")
})

test("modern release without the pinned maintainer attestation reports the actual blocker", async () => {
  for (const reviews of [[], [{ id: 12, state: "APPROVED", user: { login: "oliverhavrila", id: 28049460, type: "User" } }]]) {
    const f = fixture()
    f.reviews.splice(0, f.reviews.length, ...reviews)
    f.pr.state = "closed"
    f.pr.merged_at = "2026-09-15T16:59:53Z"
    await assert.rejects(verifyReleaseMaintainerReview(repo, f.pr, { get: f.get }),
      /missing the required exact-head generated-release attestation from oliverkko \(309485696\) before merge/)
  }
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
