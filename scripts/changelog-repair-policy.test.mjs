import assert from "node:assert/strict"
import test from "node:test"
import { auditedPendingPatchRepair } from "./changelog-repair-policy.mjs"

const repository = "darkarmy-cyber/darkphish"
const base = "cafdcc08b9d968931cb1446891055e3256d0caec"
const branch = "fix/recover-v0.20.1-review-provenance"
const files = [
  ".github/scripts/release-recover-0201-policy.mjs",
  ".github/scripts/release-recover-0201-policy.test.mjs",
  ".github/scripts/release-recover.mjs",
  "changes/recover-v0-20-1-review-provenance.md",
  "docs/RELEASE_REPAIR_0201.md",
  "scripts/changelog-repair-policy.mjs",
  "scripts/changelog-repair-policy.test.mjs",
  "scripts/changelog.mjs",
  "scripts/changelog.test.mjs",
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

test("only PR118 may stage the exact run-manifest follow-up", () => {
  const followupBase = "3cc011581698b73c76512f155f7fd78399345efd"
  const followupFiles = [
    ".github/scripts/release-recover-0201-policy.mjs", ".github/scripts/release-recover-0201-policy.test.mjs",
    "changes/recover-v0-20-1-run-manifest.md", "docs/RELEASE_REPAIR_0201.md",
    "scripts/changelog-repair-policy.mjs", "scripts/changelog-repair-policy.test.mjs",
    "scripts/release-repair-policy-0201.test.mjs", "scripts/release-repair-policy.mjs",
  ]
  const value = { repository, eventName: "pull_request", current: "0.20.1", target: "0.20.2",
    baseSHA: followupBase, files: followupFiles, event: { pull_request: { number: 118, state: "open", draft: false,
      base: { ref: "main", sha: followupBase }, head: { ref: "fix/recover-v0.20.1-run-manifest", repo: { full_name: repository } } } } }
  assert.equal(auditedPendingPatchRepair(value), true)
})

test("only PR120 may stage its 0.20.3 fragment while v0.20.2 recovery is pending", () => {
  const repairBase = "4be0bc0d1a6a2f158da5aaed996dcd87bab3bd80"
  const repairFiles = [
    ".github/scripts/release-recover-0202-policy.mjs", ".github/scripts/release-recover-0202-policy.test.mjs",
    ".github/scripts/release-recover.mjs", "changes/recover-v0-20-2-review-provenance.md",
    "docs/RELEASE_REPAIR_0202.md", "scripts/changelog-repair-policy.mjs",
    "scripts/changelog-repair-policy.test.mjs", "scripts/release-maintainer-review-0202.test.mjs",
    "scripts/release-maintainer-review.mjs", "scripts/release-repair-policy-0202.test.mjs",
    "scripts/release-repair-policy.mjs",
  ]
  const value = { repository, eventName: "pull_request", current: "0.20.2", target: "0.20.3",
    baseSHA: repairBase, files: repairFiles, event: { pull_request: { number: 120, state: "open", draft: false,
      base: { ref: "main", sha: repairBase }, head: { ref: "fix/recover-v0.20.2-review-provenance", repo: { full_name: repository } } } } }
  assert.equal(auditedPendingPatchRepair(value), true)
})

test("only PR129 may stage its 0.22.0 fragment while v0.21.1 recovery is pending", () => {
  const repairBase = "d58b43260b1b94a8a7ad6897636f9d0fb9ee06d4"
  const repairFiles = [
    ".github/scripts/release-recover-0211-policy.mjs",
    ".github/scripts/release-recover-0211-policy.test.mjs",
    ".github/scripts/release-recover.mjs",
    "changes/recover-v0-21-1-review-provenance.md",
    "scripts/changelog-repair-policy.mjs",
    "scripts/changelog-repair-policy.test.mjs",
    "scripts/release-maintainer-review-0211.test.mjs",
    "scripts/release-maintainer-review.mjs",
    "scripts/release-repair-policy-0211.test.mjs",
    "scripts/release-repair-policy.mjs",
  ]
  const fixture = () => ({
    repository, eventName: "pull_request", current: "0.21.1", target: "0.22.0",
    baseSHA: repairBase, files: [...repairFiles], event: { pull_request: {
      number: 129, state: "open", draft: false, base: { ref: "main", sha: repairBase },
      head: { ref: "fix/v0.21.1-review-recovery", repo: { full_name: repository } },
    } },
  })
  assert.equal(auditedPendingPatchRepair(fixture()), true)
  for (const change of [
    value => { value.event.pull_request.number = 130 },
    value => { value.event.pull_request.state = "closed" },
    value => { value.event.pull_request.draft = true },
    value => { value.event.pull_request.base.sha = "a".repeat(40) },
    value => { value.event.pull_request.head.ref = "fix/other" },
    value => { value.files.pop() },
    value => { value.files.push("scripts/release-publish.mjs") },
  ]) {
    const value = fixture()
    change(value)
    assert.equal(auditedPendingPatchRepair(value), false)
  }
})

test("only PR135 may stage its exact v0.22.0 recovery hardening scope", () => {
  const repairBase = "69355444d01c7ef29709ec398fdbacfc4d9b4753"
  const repairFiles = [
    ".github/scripts/release-recover.mjs",
    ".github/scripts/release-recover.test.mjs",
    "changes/harden-v0-21-1-recovery.md",
    "scripts/changelog-repair-policy.mjs",
    "scripts/changelog-repair-policy.test.mjs",
    "scripts/release-repair-policy-0211.test.mjs",
    "scripts/release-repair-policy.mjs",
  ]
  const fixture = () => ({
    repository, eventName: "pull_request", current: "0.21.1", target: "0.22.0",
    baseSHA: repairBase, files: [...repairFiles], event: { pull_request: {
      number: 135, state: "open", draft: false, base: { ref: "main", sha: repairBase },
      head: { ref: "fix/v0.21.1-recovery-hardening", repo: { full_name: repository } },
    } },
  })
  assert.equal(auditedPendingPatchRepair(fixture()), true)
  for (const change of [
    value => { value.event.pull_request.number = 136 },
    value => { value.baseSHA = "a".repeat(40) },
    value => { value.event.pull_request.base.sha = "a".repeat(40) },
    value => { value.files.pop() },
    value => { value.files.push("VERSION") },
  ]) {
    const value = fixture(); change(value)
    assert.equal(auditedPendingPatchRepair(value), false)
  }
})

test("only the immediate reviewed squash merge may keep pending patch validation green", () => {
  const headSHA = "d".repeat(40)
  const value = {
    repository, eventName: "push", current: "0.20.1", target: "0.20.2", baseSHA: base,
    files: [...files], headSHA, parentSHAs: [base],
    commitTitle: "fix(release): recover v0.20.1 review provenance (#117)",
    event: { ref: "refs/heads/main", before: base, after: headSHA, deleted: false, forced: false,
      repository: { full_name: repository } },
  }
  assert.equal(auditedPendingPatchRepair(value), true)
  for (const change of [
    candidate => { candidate.event.before = "a".repeat(40) }, candidate => { candidate.event.after = "b".repeat(40) },
    candidate => { candidate.parentSHAs.push("c".repeat(40)) }, candidate => { candidate.commitTitle = "other" },
    candidate => { candidate.files.pop() }, candidate => { candidate.event.forced = true },
  ]) { const candidate = structuredClone(value); change(candidate); assert.equal(auditedPendingPatchRepair(candidate), false) }
})

test("pending-patch repair rejects any identity, version, base or file drift", () => {
  const changes = [
    value => { value.repository = "other/repo" },
    value => { value.eventName = "workflow_dispatch" },
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

test("only PR203 may stage the exact v0.25.2 repair while v0.25.1 is unpublished", () => {
  const repairBase = "98d96c7cc7eebf6ab09bbeffdf109778dd4f3632"
  const repairFiles = [
    "changes/braces-3-0-4-audit.md",
    "docs/RELEASE_REPAIR_0251.md",
    "pnpm-workspace.yaml",
    "scripts/changelog-repair-policy.mjs",
    "scripts/changelog-repair-policy.test.mjs",
    "scripts/release-pending.mjs",
    "scripts/release-pending.test.mjs",
    "scripts/release-repair-policy.mjs",
    "scripts/release-repair-policy.test.mjs",
  ]
  const fixture = () => ({
    repository, eventName: "pull_request", current: "0.25.1", target: "0.25.2",
    baseSHA: repairBase, files: [...repairFiles], event: { pull_request: {
      number: 203, state: "open", draft: false, base: { ref: "main", sha: repairBase },
      head: { ref: "fix/braces-audit-0.25.1", repo: { full_name: repository } },
    } },
  })
  assert.equal(auditedPendingPatchRepair(fixture()), true)
  for (const change of [
    value => { value.event.pull_request.number = 204 },
    value => { value.baseSHA = "a".repeat(40) },
    value => { value.event.pull_request.head.ref = "fix/other" },
    value => { value.files.pop() },
    value => { value.files.push("VERSION") },
  ]) {
    const value = fixture(); change(value)
    assert.equal(auditedPendingPatchRepair(value), false)
  }
})

test("PR205 and only its exact merge may continue scheduled v0.25.2 preparation", () => {
  const repairBase = "4e24f93277cfc5c14fbfec63a797436018c495f1"
  const repairFiles = [
    "changes/release-prepare-0252-schedule.md",
    "docs/RELEASE_REPAIR_0251.md",
    "scripts/changelog-repair-policy.mjs",
    "scripts/changelog-repair-policy.test.mjs",
    "scripts/changelog.mjs",
    "scripts/release-repair-policy.mjs",
    "scripts/release-repair-policy.test.mjs",
  ]
  const pull = {
    repository, eventName: "pull_request", current: "0.25.1", target: "0.25.2",
    baseSHA: repairBase, files: [...repairFiles], event: { pull_request: {
      number: 205, state: "open", draft: false, base: { ref: "main", sha: repairBase },
      head: { ref: "fix/release-0252-scheduled-prepare", repo: { full_name: repository } },
    } },
  }
  assert.equal(auditedPendingPatchRepair(pull), true)

  const scheduled = {
    repository, eventName: "schedule", current: "0.25.1", target: "0.25.2",
    baseSHA: repairBase, headSHA: "e".repeat(40), parentSHAs: [repairBase], scheduledPRVerified: true,
    commitTitle: "fix(release): complete v0.25.2 scheduled preparation bridge (#205)",
    files: [...repairFiles], event: {},
  }
  assert.equal(auditedPendingPatchRepair(scheduled), true)
  assert.equal(auditedPendingPatchRepair({ ...scheduled, eventName: "workflow_run" }), true)
  for (const change of [
    value => { value.baseSHA = "a".repeat(40) },
    value => { value.parentSHAs = ["b".repeat(40)] },
    value => { value.scheduledPRVerified = false },
    value => { value.commitTitle = "other" },
    value => { value.files = value.files.slice(1) },
    value => { value.eventName = "schedule"; value.current = "0.25.0" },
  ]) {
    const value = structuredClone(scheduled); change(value)
    assert.equal(auditedPendingPatchRepair(value), false)
  }
})
