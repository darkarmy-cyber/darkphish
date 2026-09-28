import assert from "node:assert/strict"
import test from "node:test"
import { verifyReleaseRepair } from "./release-repair-policy.mjs"

const repo = "darkarmy-cyber/darkphish"
const base = "4be0bc0d1a6a2f158da5aaed996dcd87bab3bd80"
const head = "a".repeat(40)
const allowed = [
  ".github/scripts/release-recover-0202-policy.mjs",
  ".github/scripts/release-recover-0202-policy.test.mjs",
  ".github/scripts/release-recover.mjs",
  "scripts/changelog-repair-policy.mjs",
  "scripts/changelog-repair-policy.test.mjs",
  "scripts/release-maintainer-review.mjs",
  "scripts/release-maintainer-review-0202.test.mjs",
  "scripts/release-repair-policy.mjs",
  "scripts/release-repair-policy-0202.test.mjs",
  "changes/recover-v0-20-2-review-provenance.md",
  "docs/RELEASE_REPAIR_0202.md",
]
const added = new Set([
  ".github/scripts/release-recover-0202-policy.mjs",
  ".github/scripts/release-recover-0202-policy.test.mjs",
  "scripts/release-maintainer-review-0202.test.mjs",
  "scripts/release-repair-policy-0202.test.mjs",
  "changes/recover-v0-20-2-review-provenance.md",
  "docs/RELEASE_REPAIR_0202.md",
])

function fixture() {
  const pr = {
    number: 120, state: "open", draft: false, author_association: "OWNER",
    head: { sha: head, ref: "fix/recover-v0.20.2-review-provenance", repo: { full_name: repo } },
    base: { ref: "main", sha: base },
  }
  const files = allowed.map(filename => ({ filename, status: added.has(filename) ? "added" : "modified" }))
  const request = async (path, options = {}) => {
    if (path.endsWith("/files?per_page=100&page=1")) return structuredClone(files)
    if (path.includes("/contents/VERSION?")) return { type: "file", encoding: "base64", content: Buffer.from("0.20.2\n").toString("base64") }
    if (path.endsWith("/git/ref/tags/v0.20.2") || path.endsWith("/releases/tags/v0.20.2")) {
      assert.equal(options.missing, true)
      return null
    }
    throw new Error(`Unexpected request ${path}`)
  }
  return { pr, files, request, verify: () => verifyReleaseRepair(repo, pr, request) }
}

test("PR120 alone may repair the absent v0.20.2 release provenance boundary", async () => {
  await fixture().verify()
})

test("v0.20.2 repair rejects scope, identity, version and release-state drift", async () => {
  const changes = [
    f => { f.pr.number = 121 },
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
      if (target === "version" && path.includes("/contents/VERSION?")) return { type: "file", encoding: "base64", content: Buffer.from("0.20.3\n").toString("base64") }
      if (target === "tag" && path.endsWith("/git/ref/tags/v0.20.2")) return { object: { type: "commit", sha: base } }
      if (target === "release" && path.endsWith("/releases/tags/v0.20.2")) return { tag_name: "v0.20.2" }
      return original(path, options)
    }
    await assert.rejects(verifyReleaseRepair(repo, f.pr, f.request))
  }
})
