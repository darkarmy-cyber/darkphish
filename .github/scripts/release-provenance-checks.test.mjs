import assert from "node:assert/strict"
import test from "node:test"
import { canonicalMainCheckRun, nativeReleaseAttemptStart } from "./release-provenance-checks.mjs"

const source = "5a39984942ca8920aca2bf2fc31ca3201a4173be"
const bot = { login: "github-actions[bot]", type: "Bot", id: 41898282 }
const ci = { name: "CI", path: ".github/workflows/ci.yml", event: "workflow_dispatch", actor: bot,
  head_branch: "main", head_sha: source, status: "completed", conclusion: "success", updated_at: "2026-09-29T19:08:29Z" }
const codeql = { ...ci, name: "CodeQL", path: ".github/workflows/codeql.yml", event: "repository_dispatch", updated_at: "2026-09-29T19:04:10Z" }
const run = { id: 36616244777, run_attempt: 2, head_sha: source, created_at: "2026-09-29T19:01:14Z" }
const metadata = { name: "metadata", run_id: run.id, run_attempt: 2, head_sha: source,
  started_at: "2026-09-29T19:12:18Z", completed_at: "2026-09-29T19:12:36Z" }

test("exact-source dispatched checks qualify before the retried native release attempt", () => {
  const before = nativeReleaseAttemptStart(run, metadata)
  assert.equal(canonicalMainCheckRun(ci, "CI", source, before), true)
  assert.equal(canonicalMainCheckRun(codeql, "CodeQL", source, before), true)
  assert.equal(canonicalMainCheckRun(ci, "CI", source, Date.parse(run.created_at)), false)
  assert.equal(canonicalMainCheckRun({ ...ci, event: "push", actor: { login: "oliverkko" } }, "CI", source, before), true)
})

test("dispatched checks reject untrusted actors, wrong events, source, workflow, or timing", () => {
  const before = nativeReleaseAttemptStart(run, metadata)
  for (const changed of [
    { actor: { login: "oliverkko" } }, { actor: { ...bot, id: 1 } }, { event: "pull_request" },
    { head_sha: "a".repeat(40) }, { head_branch: "feature" }, { path: ".github/workflows/other.yml" },
    { conclusion: "failure" }, { updated_at: "2026-09-29T19:12:19Z" }, { updated_at: "invalid" },
  ]) assert.equal(canonicalMainCheckRun({ ...ci, ...changed }, "CI", source, before), false)
  assert.equal(canonicalMainCheckRun(ci, "CodeQL", source, before), false)
  assert.equal(canonicalMainCheckRun(codeql, "CI", source, before), false)
})

test("attempt chronology is bound to the exact run, retry, and source", () => {
  assert.equal(nativeReleaseAttemptStart(run, metadata), Date.parse(metadata.started_at))
  for (const changed of [
    { run_id: 1 }, { run_attempt: 1 }, { head_sha: "a".repeat(40) },
    { started_at: "2026-09-29T19:00:00Z" }, { completed_at: "2026-09-29T19:12:17Z" },
    { started_at: "invalid" },
  ]) assert.throws(() => nativeReleaseAttemptStart(run, { ...metadata, ...changed }))
})
