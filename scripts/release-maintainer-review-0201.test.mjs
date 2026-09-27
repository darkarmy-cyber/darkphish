import assert from "node:assert/strict"
import test from "node:test"
import { verifyReleaseMaintainerReview } from "./release-maintainer-review.mjs"

const repo = "darkarmy-cyber/darkphish"
const head = "5bd47cde0983e0506fd1dc177efe59cfd1960ef9"
const base = "1ad4467eaca1f6d182e4f0458f39cda58e5d3ce6"
const merge = "cafdcc08b9d968931cb1446891055e3256d0caec"
const bot = { login: "github-actions[bot]", id: 41898282, type: "Bot" }
const owner = { login: "oliverkko", id: 309485696, type: "User" }

function fixture() {
  const pr = {
    number: 116, state: "closed", draft: false, merged_at: "2026-09-27T19:02:26Z", merge_commit_sha: merge,
    merged_by: { ...owner }, title: "release: Darkphish 0.20.1", user: { ...bot },
    head: { ref: "release/v0.20.1", sha: head, repo: { full_name: repo } },
    base: { ref: "main", sha: base },
  }
  const files = [
    { filename: "CHANGELOG.md", status: "modified", sha: "77d0215697b09b5cc9b5a6f65be566694504f64d", additions: 12, deletions: 0 },
    { filename: "VERSION", status: "modified", sha: "847e9aef6d1fc26617ae67d57b2cf5f920bd5efe", additions: 1, deletions: 1 },
    { filename: "changes/recover-existing-release-tag.md", status: "removed", sha: "378f523d005050ae0d47b1fdaa61421d71c729e8", additions: 0, deletions: 5 },
    { filename: "changes/recovery-redacted-ruleset.md", status: "removed", sha: "17dc7f6a4986b0cc2161432c4f9e0f07cddf6dd1", additions: 0, deletions: 6 },
    { filename: "changes/recovery-ruleset-context.md", status: "removed", sha: "6b18d987b1c92d0f10d6b6f6918ebfc223c57f03", additions: 0, deletions: 6 },
    { filename: "changes/security-hardening-0-20.md", status: "removed", sha: "6dcb68fd06cb619b758f98025a44c6f8cc9de78a", additions: 0, deletions: 5 },
  ]
  const threads = []
  const finalPR = structuredClone(pr)
  const get = async (path, options = {}) => {
    if (path.startsWith(`repos/${repo}/pulls/116/files?`)) return structuredClone(files)
    if (path.startsWith(`repos/${repo}/pulls/116/reviews?`)) return []
    if (path === `repos/${repo}/pulls/116`) return structuredClone(finalPR)
    if (path === "graphql" && options.body?.query?.includes("reviewThreads")) return { data: { repository: { pullRequest: {
      headRefOid: head,
      reviewThreads: { nodes: structuredClone(threads), pageInfo: { hasNextPage: false, endCursor: null } },
    } } } }
    throw new Error(`Unexpected path ${path}`)
  }
  return { pr, files, threads, finalPR, get, verify: () => verifyReleaseMaintainerReview(repo, pr, { get }) }
}

test("only the immutable generated v0.20.1 merge receives the audited recovery authorization", async () => {
  const f = fixture(), evidence = await f.verify()
  assert.equal(evidence.auditedRelease0201, true)
  assert.equal(evidence.head, head)
  assert.equal(evidence.base, base)
  assert.equal(evidence.mergeCommit, merge)
})

test("v0.20.1 recovery fails closed on identity, tree, chronology or review-state drift", async () => {
  const changes = [
    f => { f.pr.number = 115 },
    f => { f.pr.head.ref = "release/v0.20.2"; f.pr.title = "release: Darkphish 0.20.2" },
    f => { f.pr.head.sha = "a".repeat(40) },
    f => { f.pr.base.sha = "b".repeat(40) },
    f => { f.pr.merge_commit_sha = "c".repeat(40) },
    f => { f.pr.merged_at = "2026-09-27T19:02:27Z" },
    f => { f.pr.merged_by = { ...bot } },
    f => { f.files[0].sha = "d".repeat(40) },
    f => { f.files[0].status = "added" },
    f => { f.files[0].additions = 13 },
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
