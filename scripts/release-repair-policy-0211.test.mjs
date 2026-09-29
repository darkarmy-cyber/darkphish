import assert from "node:assert/strict"
import test from "node:test"
import { verifyReleaseRepair } from "./release-repair-policy.mjs"

const repo = "darkarmy-cyber/darkphish"
const base = "d58b43260b1b94a8a7ad6897636f9d0fb9ee06d4"
const head = "a".repeat(40)
const allowed = [
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
const added = new Set([
  ".github/scripts/release-recover-0211-policy.mjs",
  ".github/scripts/release-recover-0211-policy.test.mjs",
  "changes/recover-v0-21-1-review-provenance.md",
  "scripts/release-maintainer-review-0211.test.mjs",
  "scripts/release-repair-policy-0211.test.mjs",
])

function fixture() {
  const pr = {
    number: 129, state: "open", draft: false, author_association: "OWNER",
    head: { sha: head, ref: "fix/v0.21.1-review-recovery", repo: { full_name: repo } },
    base: { ref: "main", sha: base },
  }
  const files = allowed.map(filename => ({ filename, status: added.has(filename) ? "added" : "modified" }))
  const request = async (path, options = {}) => {
    if (path.endsWith("/files?per_page=100&page=1")) return structuredClone(files)
    if (path.includes("/contents/VERSION?")) return { type: "file", encoding: "base64", content: Buffer.from("0.21.1\n").toString("base64") }
    if (path.endsWith("/git/ref/tags/v0.21.1") || path.endsWith("/releases/tags/v0.21.1")) {
      assert.equal(options.missing, true)
      return null
    }
    throw new Error(`Unexpected request ${path}`)
  }
  return { pr, files, request, verify: () => verifyReleaseRepair(repo, pr, request) }
}

test("PR129 alone may repair the absent v0.21.1 release provenance boundary", async () => {
  await fixture().verify()
})

test("v0.21.1 repair rejects scope, identity, version and release-state drift", async () => {
  const changes = [
    f => { f.pr.number = 130 },
    f => { f.pr.base.sha = "b".repeat(40) },
    f => { f.pr.head.ref = "fix/other" },
    f => { f.pr.head.repo.full_name = "other/repo" },
    f => { f.pr.author_association = "NONE" },
    f => { f.files[0].status = "removed" },
    f => { f.files.push({ filename: "VERSION", status: "modified" }) },
  ]
  for (const change of changes) {
    const f = fixture(); change(f)
    await assert.rejects(f.verify())
  }

  for (const target of ["tag", "release", "version"]) {
    const f = fixture(), original = f.request
    f.request = async (path, options) => {
      if (target === "version" && path.includes("/contents/VERSION?")) return { type: "file", encoding: "base64", content: Buffer.from("0.22.0\n").toString("base64") }
      if (target === "tag" && path.endsWith("/git/ref/tags/v0.21.1")) return { object: { type: "commit", sha: base } }
      if (target === "release" && path.endsWith("/releases/tags/v0.21.1")) return { tag_name: "v0.21.1" }
      return original(path, options)
    }
    await assert.rejects(verifyReleaseRepair(repo, f.pr, f.request))
  }
})
