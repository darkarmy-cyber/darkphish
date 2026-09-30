import assert from "node:assert/strict"
import test from "node:test"
import { audited0222MetadataFailure } from "./release-recover-0222-policy.mjs"

const repository = "darkarmy-cyber/darkphish"
const source = "743633afd812980852480cf746a168c75814cde5"

function runFixture() {
  const step = (number, name, conclusion) => ({ number, name, status: "completed", conclusion })
  const job = (id, name, conclusion, started_at = "2026-09-30T12:58:35Z") => ({
    id, run_id: 36717602969, run_attempt: 2, head_sha: source, name,
    status: "completed", conclusion, started_at, completed_at: "2026-09-30T12:58:35Z",
  })
  const metadata = job(109896607865, "metadata", "failure", "2026-09-30T12:58:16Z")
  metadata.completed_at = "2026-09-30T12:58:34Z"
  metadata.steps = [
    step(1, "Set up job", "success"), step(2, "Run actions/checkout@v7", "success"),
    step(3, "Run actions/setup-node@v7", "success"), step(4, "Run node scripts/changelog.mjs validate", "success"),
    step(5, "Enforce release normalization hold", "success"), step(6, "Run node scripts/release-publish.mjs metadata", "failure"),
    step(11, "Post Run actions/setup-node@v7", "skipped"), step(12, "Post Run actions/checkout@v7", "success"),
    step(13, "Complete job", "success"),
  ]
  return { repository, source, run: {
    id: 36717602969, run_number: 432, run_attempt: 2, workflow_id: 351159930,
    name: "Native release", path: ".github/workflows/release.yml", event: "workflow_run",
    head_branch: "main", head_sha: source, status: "completed", conclusion: "failure",
    created_at: "2026-09-30T12:52:13Z", updated_at: "2026-09-30T12:58:36Z",
    actor: { login: "oliverkko", id: 309485696, type: "User" },
  }, jobs: [
    metadata,
    job(109896762661, "verify", "skipped"),
    job(109896762934, "binaries", "skipped"),
    job(109896763770, "audit-smoke", "skipped"),
    job(109896765002, "publish", "skipped"),
  ] }
}

test("historical provenance accepts only Native release run 432 attempt 2 exact metadata failure", () => {
  assert.equal(audited0222MetadataFailure(runFixture()), true)
  for (const change of [
    value => { value.repository = "other/repo" },
    value => { value.run.head_sha = "b".repeat(40) },
    value => { value.run.event = "workflow_dispatch" },
    value => { value.run.path = ".github/workflows/other.yml" },
    value => { value.jobs[4] = structuredClone(value.jobs[3]) },
    value => { value.jobs[0].steps[0].conclusion = "failure" },
    value => { value.jobs[0].steps.pop() },
    value => { value.source = "a".repeat(40) }, value => { value.run.id++ },
    value => { value.run.run_attempt = 1 }, value => { value.run.workflow_id++ },
    value => { value.run.updated_at = "2026-09-29T10:49:36Z" }, value => { value.run.actor.id++ },
    value => { value.jobs[0].run_attempt = 1 }, value => { value.jobs[0].head_sha = "b".repeat(40) },
    value => { value.jobs[0].steps[5].conclusion = "success" },
    value => { value.jobs[1].conclusion = "success" }, value => { value.jobs.pop() },
  ]) { const value = runFixture(); change(value); assert.equal(audited0222MetadataFailure(value), false) }
})
