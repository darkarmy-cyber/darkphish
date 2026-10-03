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


test("allows only the exact audited tagless v0.23.1 state to bridge to v0.24.0", async () => {
  let verifyCalls = 0
  const verify = async () => { verifyCalls += 1; throw new Error("must not verify unpublished v0.23.1") }
  const pr = {
    number: 181, state: "closed", draft: false, title: "release: Darkphish 0.23.1",
    merged_at: "2026-10-02T12:24:22Z", merge_commit_sha: "86cb7173c553d68bc3afac204948aa923a32bc83",
    base: { ref: "main", sha: "443d247b1636ea8e46c7339378174d3c88a0abf5" },
    head: { ref: "release/v0.23.1", sha: "5f8b6de5a6ed4076b987747c32d615b5b9bd180a", repo: { full_name: "darkarmy-cyber/darkphish" } },
  }
  const request = async (path) => path.endsWith("/pulls/181") ? structuredClone(pr) : null
  await assertReleaseAdvancePublished("darkarmy-cyber/darkphish", "0.23.1", "0.24.0", { verify, request })
  assert.equal(verifyCalls, 0)

  for (const mutate of [
    value => { value.merge_commit_sha = "a".repeat(40) },
    value => { value.head.sha = "b".repeat(40) },
    value => { value.base.sha = "c".repeat(40) },
  ]) {
    const changed = structuredClone(pr); mutate(changed)
    await assert.rejects(
      assertReleaseAdvancePublished("darkarmy-cyber/darkphish", "0.23.1", "0.24.0", {
        verify,
        request: async (path) => path.endsWith("/pulls/181") ? changed : null,
      }),
      /audited v0\.23\.1 bridge state changed/,
    )
  }

  await assert.rejects(
    assertReleaseAdvancePublished("darkarmy-cyber/darkphish", "0.23.1", "0.24.0", {
      verify,
      request: async (path) => path.includes("/git/ref/tags/") ? { object: { sha: "d".repeat(40) } } : path.endsWith("/pulls/181") ? pr : null,
    }),
    /audited v0\.23\.1 bridge state changed/,
  )

  await assert.rejects(
    assertReleaseAdvancePublished("darkarmy-cyber/darkphish", "0.23.1", "0.24.1", { verify, request }),
    /must not verify unpublished v0\.23\.1/,
  )
})

test("allows only the exact audited tagless v0.25.1 state to bridge to v0.25.2", async () => {
  let verifyCalls = 0
  const verify = async () => { verifyCalls += 1; throw new Error("must not verify unpublished v0.25.1") }
  const pr = {
    number: 196, state: "closed", draft: false, title: "release: Darkphish 0.25.1",
    merged_at: "2026-10-02T21:03:30Z", merge_commit_sha: "98d96c7cc7eebf6ab09bbeffdf109778dd4f3632",
    base: { ref: "main", sha: "ffba3cab315c4580695d69c72ed0539e8b6a1d46" },
    head: { ref: "release/v0.25.1", sha: "d64cd123501fd2d7b5976996a2ddef072d96b041", repo: { full_name: repo } },
  }
  const request = async path => path.endsWith("/pulls/196") ? structuredClone(pr) : null
  await assertReleaseAdvancePublished(repo, "0.25.1", "0.25.2", { verify, request })
  assert.equal(verifyCalls, 0)
  for (const mutate of [
    value => { value.merge_commit_sha = "a".repeat(40) },
    value => { value.head.sha = "b".repeat(40) },
    value => { value.base.sha = "c".repeat(40) },
  ]) {
    const changed = structuredClone(pr); mutate(changed)
    await assert.rejects(
      assertReleaseAdvancePublished(repo, "0.25.1", "0.25.2", { verify,
        request: async path => path.endsWith("/pulls/196") ? changed : null }),
      /audited v0\.25\.1 bridge state changed/,
    )
  }
})
