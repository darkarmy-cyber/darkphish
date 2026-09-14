import assert from "node:assert/strict"
import test from "node:test"
import { verifyReleaseMaintainerReview } from "./release-maintainer-review.mjs"

const repo = "darkarmy-cyber/darkphish"
const head = "a".repeat(40), base = "b".repeat(40), merge = "c".repeat(40)
const bot = { login: "github-actions[bot]", id: 41898282, type: "Bot" }
const legacyPolicy = `
export function protectedMergeArguments(repo, pr) {
  return ["pr", "merge", String(pr.number), "--repo", repo, "--auto", "--squash", "--match-head-commit", pr.head.sha]
}
export async function enableAutoMerge(repo, pr) {
  if (!metadata.allow_auto_merge || !base.protected) throw new Error("native auto-merge requires enabled repository auto-merge and a protected base branch")
  if (pr.draft || pr.head.repo?.full_name !== repo) throw new Error("only internal ready pull requests are eligible")
}
`

function fixture(policy = legacyPolicy) {
  const pr = {
    number: 20, state: "closed", draft: false,
    merged_at: "2026-09-07T08:29:08Z", merge_commit_sha: merge,
    title: "release: Darkphish 0.7.0", user: { ...bot },
    head: { ref: "release/v0.7.0", sha: head, repo: { full_name: repo } },
    base: { ref: "main", sha: base },
  }
  const files = [
    { filename: "VERSION", sha: "1".repeat(40) },
    { filename: "CHANGELOG.md", sha: "2".repeat(40) },
    { filename: "changes/release-fix.md", sha: "3".repeat(40) },
  ]
  const reviews = []
  const get = async (path, options = {}) => {
    if (path.startsWith(`repos/${repo}/pulls/20/files?`)) return structuredClone(files)
    if (path.startsWith(`repos/${repo}/pulls/20/reviews?`)) return structuredClone(reviews)
    if (path === `repos/${repo}/contents/scripts/release-lib.mjs?ref=${merge}`) return {
      type: "file", encoding: "base64", content: Buffer.from(policy).toString("base64"),
    }
    if (path === `repos/${repo}/pulls/20`) return structuredClone(pr)
    if (path === "graphql" && (options.body?.query || "").includes("reviewThreads")) return { data: { repository: { pullRequest: {
      headRefOid: head,
      reviewThreads: { nodes: [], pageInfo: { hasNextPage: false, endCursor: null } },
    } } } }
    throw new Error(`Unexpected path ${path}`)
  }
  return { pr, files, reviews, get }
}

test("pre-review generated release is accepted only from immutable protected exact-head auto-merge policy", async () => {
  const f = fixture()
  const evidence = await verifyReleaseMaintainerReview(repo, f.pr, { get: f.get })
  assert.equal(evidence.legacyProtectedAutoMerge, true)
  assert.equal(evidence.mergeCommit, merge)
  assert.equal(evidence.head, head)
})

test("historical grandfathering rejects a source that already contains the modern review gate", async () => {
  const f = fixture(`${legacyPolicy}\nconst review = "release-maintainer-review"\n`)
  await assert.rejects(verifyReleaseMaintainerReview(repo, f.pr, { get: f.get }), /already required explicit review provenance/)
})

test("historical grandfathering rejects altered protected merge policy", async () => {
  const f = fixture(legacyPolicy.replace("--match-head-commit", "--delete-branch"))
  await assert.rejects(verifyReleaseMaintainerReview(repo, f.pr, { get: f.get }), /does not prove the protected exact-head auto-merge policy/)
})

test("historical grandfathering is unavailable when any review evidence exists", async () => {
  const f = fixture()
  f.reviews.push({ id: 1, state: "COMMENTED", commit_id: head, submitted_at: "2026-09-07T08:20:00Z", user: { login: "someone", id: 99, type: "User" }, body: "comment" })
  await assert.rejects(verifyReleaseMaintainerReview(repo, f.pr, { get: f.get }), /Missing trusted maintainer generated-release review/)
})
