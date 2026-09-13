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
    state: "APPROVED",
    commit_id: head,
    submitted_at: "2026-09-13T07:00:00Z",
    user: { ...reviewer },
    body: releaseReviewBody(repo, pr),
  }
  const files = [
    { id: 1, filename: "VERSION" },
    { id: 2, filename: "CHANGELOG.md" },
    { id: 3, filename: "changes/release-fix.md" },
  ]
  const reviews = [review]
  const get = async (path) => {
    if (path.startsWith(`repos/${repo}/pulls/31/files?`)) return structuredClone(files)
    if (path.startsWith(`repos/${repo}/pulls/31/reviews?`)) return structuredClone(reviews)
    if (path === `repos/${repo}/pulls/31`) return structuredClone(pr)
    throw new Error(`Unexpected path ${path}`)
  }
  return { pr, review, files, reviews, get }
}

test("trusted maintainer approval certifies exact generated release head and base", async () => {
  const f = fixture()
  const evidence = await verifyReleaseMaintainerReview(repo, f.pr, { get: f.get })
  assert.equal(evidence.head, head)
  assert.equal(evidence.base, base)
  assert.equal(evidence.reviewer, "oliverkko")
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
    f => { f.files.push({ id: 4, filename: "models/user.go" }) },
    f => { f.review.body = f.review.body.replace(head, "c".repeat(40)) },
    f => { f.review.body = f.review.body.replace(base, "c".repeat(40)) },
    f => { f.review.body = f.review.body.replace('"decision":"approved"', '"decision":"rejected"') },
    f => { f.reviews.push({ id: 11, state: "CHANGES_REQUESTED", commit_id: head, submitted_at: "2026-09-13T07:01:00Z", user: { login: "reviewer", id: 123, type: "User" }, body: "changes required" }) },
  ]
  for (const mutate of mutations) {
    const f = fixture(); mutate(f)
    await assert.rejects(verifyReleaseMaintainerReview(repo, f.pr, { get: f.get }), ReleaseMaintainerReviewError)
  }
})

test("review is bound to immutable PR snapshot and pre-merge chronology", async () => {
  const f = fixture()
  f.pr.merged_at = "2026-09-13T07:01:00Z"
  assert.equal((await verifyReleaseMaintainerReview(repo, f.pr, { get: f.get })).head, head)
  f.pr.merged_at = "2026-09-13T06:59:59Z"
  await assert.rejects(verifyReleaseMaintainerReview(repo, f.pr, { get: f.get }), /after merge/)

  const g = fixture()
  const get = g.get
  g.get = async (path) => path === `repos/${repo}/pulls/31`
    ? { ...structuredClone(g.pr), head: { ...g.pr.head, sha: "d".repeat(40) } }
    : get(path)
  await assert.rejects(verifyReleaseMaintainerReview(repo, g.pr, { get: g.get }), /changed during/)
})
