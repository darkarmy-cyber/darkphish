import assert from "node:assert/strict"
import test from "node:test"
import { audited0211MetadataFailure, auditedTagless0211Source } from "./release-recover-0211-policy.mjs"

const repository = "darkarmy-cyber/darkphish"
const source = "51f3c3b56df05d8a6fd165d227cb78d9e46ae821"

function prFixture() {
  return { repository, version: "0.21.1", tag: "v0.21.1", pr: {
    number: 128, state: "closed", draft: false, title: "release: Darkphish 0.21.1",
    merged_at: "2026-09-29T10:41:54Z", merge_commit_sha: source,
    base: { ref: "main", sha: "e9512fc4c564dabdc098b3bf44d3748c355d4d15" },
    head: { ref: "release/v0.21.1", sha: "e5a4f4713cbe25d05d641c8b22e8e83da3ea69a7", repo: { full_name: repository } },
  } }
}

function runFixture() {
  const step = (number, name, conclusion) => ({ number, name, status: "completed", conclusion })
  const job = (id, name, conclusion, started_at = "2026-09-29T10:49:34Z") => ({
    id, run_id: 36557608515, run_attempt: 2, head_sha: source, name,
    status: "completed", conclusion, started_at, completed_at: "2026-09-29T10:49:34Z",
  })
  const metadata = job(109371482728, "metadata", "failure", "2026-09-29T10:49:21Z")
  metadata.steps = [
    step(1, "Set up job", "success"), step(2, "Run actions/checkout@v7", "success"),
    step(3, "Run actions/setup-node@v7", "success"), step(4, "Run node scripts/changelog.mjs validate", "success"),
    step(5, "Enforce release normalization hold", "success"), step(6, "Run node scripts/release-publish.mjs metadata", "failure"),
    step(11, "Post Run actions/setup-node@v7", "skipped"), step(12, "Post Run actions/checkout@v7", "success"),
    step(13, "Complete job", "success"),
  ]
  return { repository, source, run: {
    id: 36557608515, run_number: 374, run_attempt: 2, workflow_id: 351159930,
    name: "Native release", path: ".github/workflows/release.yml", event: "workflow_run",
    head_branch: "main", head_sha: source, status: "completed", conclusion: "failure",
    created_at: "2026-09-29T10:46:12Z", updated_at: "2026-09-29T10:49:35Z",
    actor: { login: "oliverkko", id: 309485696, type: "User" },
  }, jobs: [
    metadata,
    job(109371565769, "verify", "skipped"),
    job(109371566324, "binaries", "skipped"),
    job(109371566777, "audit-smoke", "skipped"),
    job(109371567499, "publish", "skipped"),
  ] }
}

test("tagless recovery selects only the immutable generated PR128 merge", () => {
  assert.equal(auditedTagless0211Source(prFixture()), source)
  for (const change of [
    value => { value.repository = "other/repo" }, value => { value.version = "0.21.2" },
    value => { value.pr.number = 129 }, value => { value.pr.merge_commit_sha = "a".repeat(40) },
    value => { value.pr.head.sha = "b".repeat(40) }, value => { value.pr.base.sha = "c".repeat(40) },
    value => { value.pr.merged_at = "2026-09-29T10:41:55Z" },
  ]) { const value = prFixture(); change(value); assert.equal(auditedTagless0211Source(value), null) }
})

test("historical provenance accepts only Native release run 374 attempt 2 exact metadata failure", () => {
  assert.equal(audited0211MetadataFailure(runFixture()), true)
  for (const change of [
    value => { value.source = "a".repeat(40) }, value => { value.run.id++ },
    value => { value.run.run_attempt = 1 }, value => { value.run.workflow_id++ },
    value => { value.run.updated_at = "2026-09-29T10:49:36Z" }, value => { value.run.actor.id++ },
    value => { value.jobs[0].run_attempt = 1 }, value => { value.jobs[0].head_sha = "b".repeat(40) },
    value => { value.jobs[0].steps[5].conclusion = "success" },
    value => { value.jobs[1].conclusion = "success" }, value => { value.jobs.pop() },
  ]) { const value = runFixture(); change(value); assert.equal(audited0211MetadataFailure(value), false) }
})
