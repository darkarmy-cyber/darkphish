import assert from "node:assert/strict"
import test from "node:test"
import { auditedPendingPatchRepair } from "./changelog-repair-policy.mjs"

const repository = "darkarmy-cyber/darkphish"
const base = "cafdcc08b9d968931cb1446891055e3256d0caec"
const branch = "fix/recover-v0.20.1-review-provenance"
const files = [
  "changes/recover-v0-20-1-review-provenance.md",
  "docs/RELEASE_REPAIR_0201.md",
  "scripts/changelog-repair-policy.mjs",
  "scripts/changelog-repair-policy.test.mjs",
  "scripts/changelog.mjs",
  "scripts/release-maintainer-review-0201.test.mjs",
  "scripts/release-maintainer-review.mjs",
  "scripts/release-repair-policy-0201.test.mjs",
  "scripts/release-repair-policy.mjs",
]

function fixture() {
  return {
    repository, eventName: "pull_request", current: "0.20.1", target: "0.20.2", baseSHA: base, files: [...files],
    event: { pull_request: { number: 117, state: "open", draft: false,
      base: { ref: "main", sha: base }, head: { ref: branch, repo: { full_name: repository } } } },
  }
}

test("only PR117 may stage its 0.20.2 fragment while v0.20.1 recovery is pending", () => {
  assert.equal(auditedPendingPatchRepair(fixture()), true)
})

test("pending-patch repair rejects any identity, version, base or file drift", () => {
  const changes = [
    value => { value.repository = "other/repo" },
    value => { value.eventName = "push" },
    value => { value.current = "0.20.0" },
    value => { value.target = "0.20.3" },
    value => { value.baseSHA = "a".repeat(40) },
    value => { value.event.pull_request.number = 118 },
    value => { value.event.pull_request.state = "closed" },
    value => { value.event.pull_request.draft = true },
    value => { value.event.pull_request.base.ref = "other" },
    value => { value.event.pull_request.base.sha = "b".repeat(40) },
    value => { value.event.pull_request.head.ref = "fix/other" },
    value => { value.event.pull_request.head.repo.full_name = "fork/repo" },
    value => { value.files.pop() },
    value => { value.files.push("scripts/release-publish.mjs") },
    value => { value.files[0] = "scripts/release-publish.mjs" },
  ]
  for (const change of changes) {
    const value = fixture(); change(value)
    assert.equal(auditedPendingPatchRepair(value), false)
  }
})
