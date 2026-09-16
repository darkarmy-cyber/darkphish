import assert from "node:assert/strict"
import test from "node:test"
import { assertNoUnstagedRelease, assertReleaseAdvancePublished } from "./release-pending.mjs"

const repo = "darkarmy-cyber/darkphish"
const releasePR = number => ({ number, state: "closed", merged_at: "2026-09-15T16:59:53Z",
  title: "release: Darkphish 0.10.0", base: { ref: "main" },
  head: { ref: "release/v0.10.0", repo: { full_name: repo } } })

test("both minor and patch advances require verified current publication", async () => {
  for (const target of ["0.11.0", "0.10.1"]) {
    const calls = []
    await assertReleaseAdvancePublished(repo, "0.10.0", target, { verify: async (...args) => calls.push(args) })
    assert.deepEqual(calls, [[repo, { version: "0.10.0" }]])
    await assert.rejects(assertReleaseAdvancePublished(repo, "0.10.0", target, {
      verify: async () => { throw new Error("current release is unverified") },
    }), /current release is unverified/)
  }
})

test("repairing the same unpublished version remains possible", async () => {
  await assertReleaseAdvancePublished(repo, "0.10.0", "0.10.0", {
    verify: async () => { throw new Error("must not require publication of itself") },
  })
})

test("tagless and draftless merged releases are reported with every candidate PR", async () => {
  const paths = []
  await assert.rejects(assertNoUnstagedRelease(repo, "0.10.0", { request: async path => {
    paths.push(path)
    return [releasePR(69), releasePR(72)]
  } }), /v0.10.0 has merged release PRs \(#69, #72\) but no tag or staging draft/)
  assert.equal(paths.length, 1)
  assert.match(paths[0], /state=closed&base=main&head=darkarmy-cyber:release\/v0.10.0/)
})

test("closed unmerged and unrelated PRs are not unfinished release merges", async () => {
  await assertNoUnstagedRelease(repo, "0.10.0", { request: async () => [
    { ...releasePR(1), merged_at: null },
    { ...releasePR(2), title: "other work" },
    { ...releasePR(3), head: { ref: "release/v0.10.0", repo: { full_name: "fork/darkphish" } } },
  ] })
})

test("discovery errors propagate instead of reporting a successful no-op", async () => {
  await assert.rejects(assertNoUnstagedRelease(repo, "0.10.0", { request: async () => {
    throw new Error("GitHub unavailable")
  } }), /GitHub unavailable/)
})
