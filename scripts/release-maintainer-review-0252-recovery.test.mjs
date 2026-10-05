import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const source = readFileSync(new URL("./release-maintainer-review.mjs", import.meta.url), "utf8")

test("v0.25.2 recovery is pinned to the audited post-merge attestation tuple", () => {
  for (const value of [
    'pr.number === 206',
    'pr.head.ref === "release/v0.25.2"',
    'pr.head.sha === "5e10e7f894b62f84fbe0685f8cffd631bda328ed"',
    'pr.base.sha === "a9bae16aa396edd3f19856142abbd763add09674"',
    'pr.merge_commit_sha === "eb0fe18b9fde19fea159961cb9e3c863cb58b437"',
    'pr.merged_at === "2026-10-03T20:45:16Z"',
    'review.id === 5402865109',
    'attestation.submitted_at === "2026-10-03T21:40:03Z"',
    'auditedRelease0252: true',
  ]) assert.ok(source.includes(value), `missing audited v0.25.2 recovery pin: ${value}`)
})

test("v0.25.2 exception remains explicit and cannot become a generic post-merge bypass", () => {
  assert.match(source, /if \(repo === "darkarmy-cyber\/darkphish" && pr\.number === 206\) \{\s*return verifyAuditedRelease0252Merge/)
  assert.ok(source.includes('requireReview(timestamp(attestation.submitted_at) > timestamp(pr.merged_at)'))
  assert.ok(source.includes('await verifyReviewGraphQL(get, attestation)'))
  assert.ok(source.includes('await verifyResolvedThreads(get, repo, pr)'))
})
