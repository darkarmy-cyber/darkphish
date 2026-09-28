import assert from "node:assert/strict"
import test from "node:test"
import { audited0202MetadataFailure, auditedTagless0202Source } from "./release-recover-0202-policy.mjs"

const repository = "darkarmy-cyber/darkphish"
const source = "4be0bc0d1a6a2f158da5aaed996dcd87bab3bd80"

function prFixture() {
  return { repository, version: "0.20.2", tag: "v0.20.2", pr: {
    number: 119, state: "closed", draft: false, title: "release: Darkphish 0.20.2",
    merged_at: "2026-09-27T21:50:12Z", merge_commit_sha: source,
    base: { ref: "main", sha: "eef3789a3f7f23b03fd233716a7e0c5209a8c4c9" },
    head: { ref: "release/v0.20.2", sha: "64241df5e4225693105e1a5989c12d453f76f8ba", repo: { full_name: repository } },
  } }
}

function runFixture() {
  const step = (number, name, conclusion) => ({ number, name, status: "completed", conclusion })
  return { repository, source, run: {
    id: 36353391121, run_number: 352, run_attempt: 1, workflow_id: 351159930,
    name: "Native release", path: ".github/workflows/release.yml", event: "workflow_run",
    head_branch: "main", head_sha: source, status: "completed", conclusion: "failure",
    actor: { login: "oliverkko", id: 309485696, type: "User" },
  }, jobs: [
    { id: 108716317734, run_id: 36353391121, name: "metadata", status: "completed", conclusion: "failure", steps: [
      step(1, "Set up job", "success"), step(2, "Run actions/checkout@v7", "success"),
      step(3, "Run actions/setup-node@v7", "success"), step(4, "Run node scripts/changelog.mjs validate", "success"),
      step(5, "Enforce release normalization hold", "success"), step(6, "Run node scripts/release-publish.mjs metadata", "failure"),
      step(11, "Post Run actions/setup-node@v7", "skipped"), step(12, "Post Run actions/checkout@v7", "success"),
      step(13, "Complete job", "success"),
    ] },
    { id: 108716363982, run_id: 36353391121, name: "verify", status: "completed", conclusion: "skipped" },
    { id: 108716364413, run_id: 36353391121, name: "binaries", status: "completed", conclusion: "skipped" },
    { id: 108716364445, run_id: 36353391121, name: "audit-smoke", status: "completed", conclusion: "skipped" },
    { id: 108716364937, run_id: 36353391121, name: "publish", status: "completed", conclusion: "skipped" },
  ] }
}

test("tagless recovery selects only the immutable generated PR119 merge", () => {
  assert.equal(auditedTagless0202Source(prFixture()), source)
  for (const change of [
    value => { value.repository = "other/repo" }, value => { value.version = "0.20.3" },
    value => { value.pr.number = 120 }, value => { value.pr.merge_commit_sha = "a".repeat(40) },
    value => { value.pr.head.sha = "b".repeat(40) }, value => { value.pr.base.sha = "c".repeat(40) },
  ]) { const value = prFixture(); change(value); assert.equal(auditedTagless0202Source(value), null) }
})

test("historical provenance accepts only Native release run 352's exact metadata failure", () => {
  assert.equal(audited0202MetadataFailure(runFixture()), true)
  for (const change of [
    value => { value.source = "a".repeat(40) }, value => { value.run.id++ },
    value => { value.run.workflow_id++ }, value => { value.run.actor.id++ },
    value => { value.jobs[0].steps[5].conclusion = "success" },
    value => { value.jobs[1].conclusion = "success" }, value => { value.jobs.pop() },
  ]) { const value = runFixture(); change(value); assert.equal(audited0202MetadataFailure(value), false) }
})
