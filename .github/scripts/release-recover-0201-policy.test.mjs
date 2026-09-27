import assert from "node:assert/strict"
import test from "node:test"
import { audited0201MetadataFailure, auditedTagless0201Source } from "./release-recover-0201-policy.mjs"

const repository = "darkarmy-cyber/darkphish"
const source = "cafdcc08b9d968931cb1446891055e3256d0caec"

function prFixture() {
  return { repository, version: "0.20.1", tag: "v0.20.1", pr: {
    number: 116, state: "closed", draft: false, title: "release: Darkphish 0.20.1",
    merged_at: "2026-09-27T19:02:26Z", merge_commit_sha: source,
    base: { ref: "main", sha: "1ad4467eaca1f6d182e4f0458f39cda58e5d3ce6" },
    head: { ref: "release/v0.20.1", sha: "5bd47cde0983e0506fd1dc177efe59cfd1960ef9", repo: { full_name: repository } },
  } }
}

function runFixture() {
  const step = (number, name, conclusion) => ({ number, name, status: "completed", conclusion })
  return { repository, source, run: {
    id: 36343131819, run_number: 345, run_attempt: 1, workflow_id: 351159930,
    name: "Native release", path: ".github/workflows/release.yml", event: "workflow_run",
    head_branch: "main", head_sha: source, status: "completed", conclusion: "failure",
    actor: { login: "oliverkko", id: 309485696, type: "User" },
  }, jobs: [
    { id: 108687061302, run_id: 36343131819, name: "metadata", status: "completed", conclusion: "failure", steps: [
      step(1, "Set up job", "success"), step(2, "Run actions/checkout@v7", "success"),
      step(3, "Run actions/setup-node@v7", "success"), step(4, "Run node scripts/changelog.mjs validate", "success"),
      step(5, "Enforce release normalization hold", "success"), step(6, "Run node scripts/release-publish.mjs metadata", "failure"),
      step(11, "Post Run actions/setup-node@v7", "skipped"), step(12, "Post Run actions/checkout@v7", "success"),
      step(13, "Complete job", "success"),
    ] },
    { id: 108687103646, run_id: 36343131819, name: "verify", status: "completed", conclusion: "skipped" },
    { id: 108687104304, run_id: 36343131819, name: "binaries", status: "completed", conclusion: "skipped" },
    { id: 108687104319, run_id: 36343131819, name: "audit-smoke", status: "completed", conclusion: "skipped" },
    { id: 108687104828, run_id: 36343131819, name: "publish", status: "completed", conclusion: "skipped" },
  ] }
}

test("tagless recovery selects only the immutable generated PR116 merge", () => {
  assert.equal(auditedTagless0201Source(prFixture()), source)
  for (const change of [
    value => { value.repository = "other/repo" }, value => { value.version = "0.20.2" },
    value => { value.pr.number = 117 }, value => { value.pr.merge_commit_sha = "a".repeat(40) },
    value => { value.pr.head.sha = "b".repeat(40) }, value => { value.pr.base.sha = "c".repeat(40) },
  ]) { const value = prFixture(); change(value); assert.equal(auditedTagless0201Source(value), null) }
})

test("historical provenance accepts only Native release run 345's exact metadata failure", () => {
  assert.equal(audited0201MetadataFailure(runFixture()), true)
  for (const change of [
    value => { value.source = "a".repeat(40) }, value => { value.run.id++ },
    value => { value.run.workflow_id++ }, value => { value.run.actor.id++ },
    value => { value.jobs[0].steps[5].conclusion = "success" },
    value => { value.jobs[1].conclusion = "success" }, value => { value.jobs.pop() },
  ]) { const value = runFixture(); change(value); assert.equal(audited0201MetadataFailure(value), false) }
})
