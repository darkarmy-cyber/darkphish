import assert from "node:assert/strict"
import test from "node:test"
import { canonicalMainCheckRun } from "../.github/scripts/release-provenance-checks.mjs"

const source = "584b266ac824cd1b02e53c3d6a4964211703bd69"
const maintainer = {login: "oliverkko", id: 309485696, type: "User"}
const repository = {full_name: "darkarmy-cyber/darkphish", id: 1358551132, fork: false}
const before = Date.parse("2026-10-01T13:16:09Z")
const ci = {id: 36866865260, run_attempt: 1, name: "CI", path: ".github/workflows/ci.yml",
  event: "workflow_dispatch", actor: maintainer, triggering_actor: maintainer, repository,
  head_branch: "main", head_sha: source, status: "completed", conclusion: "success", updated_at: "2026-10-01T13:16:03Z"}
const codeql = {...ci, id: 36866872453, name: "CodeQL", path: ".github/workflows/codeql.yml",
  event: "repository_dispatch", updated_at: "2026-10-01T13:13:51Z"}

test("audited v0.22.3 checks predate the exact native publication attempt", () => {
  assert.equal(canonicalMainCheckRun(ci, "CI", source, before), true)
  assert.equal(canonicalMainCheckRun(codeql, "CodeQL", source, before), true)
  assert.equal(canonicalMainCheckRun(codeql, "CI", source, before), false)
})

test("audited evidence cannot authorize other runs, actors, repositories, attempts or sources", () => {
  for (const original of [ci, codeql]) {
    for (const changed of [
      {id: original.id + 1}, {run_attempt: 2}, {actor: {...maintainer, id: 1}},
      {triggering_actor: {...maintainer, login: "other"}}, {repository: {...repository, id: 1}},
      {repository: {...repository, full_name: "other/darkphish"}}, {repository: {...repository, fork: true}},
      {head_sha: "a".repeat(40)}, {head_branch: "feature"}, {event: "pull_request"},
      {path: ".github/workflows/other.yml"}, {conclusion: "failure"}, {status: "in_progress"},
      {updated_at: "2026-10-01T13:16:10Z"}, {updated_at: "invalid"},
    ]) assert.equal(canonicalMainCheckRun({...original, ...changed}, original.name, source, before), false)
    assert.equal(canonicalMainCheckRun(original, original.name, "a".repeat(40), before), false)
  }
})
