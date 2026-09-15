import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"
import { discoverRelease, assertStagingMetadata } from "./release-discovery.mjs"
import { assertReleaseState } from "./release-lib.mjs"
import { releaseBody } from "./release-notes.mjs"

const repo = "owner/repository", tag = "v0.9.0", sha = "a".repeat(40)
const base = `repos/${repo}/releases`
const body = releaseBody("## 0.9.0 - 2026-09-15\n\n### Fixed\n\n- Draft discovery.", "0.9.0", sha)
const draft = { id: 123, tag_name: tag, draft: true, prerelease: false, target_commitish: sha,
  name: "Darkphish 0.9", body, author: { login: "github-actions[bot]", type: "Bot", id: 41898282 } }

function fixture({ byTag = null, listing = [draft], direct = draft, next = [] } = {}) {
  const calls = []
  const request = async (path, options) => {
    calls.push(path)
    assert.equal(options?.method, undefined, "discovery must remain read-only")
    if (path === `${base}/tags/${tag}`) { assert.equal(options.missing, true); return byTag }
    if (path === `${base}?per_page=100&page=1`) return listing
    if (path === `${base}?per_page=100&page=2`) return next
    if (path === `${base}/${draft.id}`) return direct
    throw new Error(`unexpected request ${path}`)
  }
  return { request, calls }
}

test("by-tag 404 still discovers a trusted staging draft and resumes, never creates another", async () => {
  const f = fixture()
  const found = await discoverRelease(repo, tag, f)
  assert.equal(found, draft)
  assert.equal(assertReleaseState(null, found, sha), "resume")
  assertStagingMetadata(found, { tag, name: draft.name, body })
  assert.deepEqual(f.calls, [`${base}/tags/${tag}`, `${base}?per_page=100&page=1`, `${base}/123`])
})

test("a published by-tag identity remains authoritative despite retained historical private drafts", async () => {
  const published = { ...draft, draft: false, id: 456 }
  const f = fixture({ byTag: published })
  assert.equal(await discoverRelease(repo, tag, f), published)
  assert.equal(f.calls.length, 1)
})

test("full pagination finds drafts beyond page one and rejects cross-page ambiguity", async () => {
  const unrelated = Array.from({ length: 100 }, (_, id) => ({ id: id + 1000, tag_name: `v1.0.${id}` }))
  const f = fixture({ listing: unrelated, next: [draft] })
  assert.equal(await discoverRelease(repo, tag, f), draft)
  await assert.rejects(discoverRelease(repo, tag, fixture({ listing: [draft, ...unrelated.slice(1)], next: [{ ...draft, id: 456 }] })), /multiple release candidates/)
})

test("absence is new but same-tag duplicates and endpoint disagreement fail closed", async () => {
  assert.equal(await discoverRelease(repo, tag, fixture({ listing: [] })), null)
  await assert.rejects(discoverRelease(repo, tag, fixture({ listing: [draft, draft] })), /multiple release candidates/)
  await assert.rejects(discoverRelease(repo, tag, fixture({ byTag: draft, listing: [] })), /disappeared/)
  await assert.rejects(discoverRelease(repo, tag, fixture({ byTag: { ...draft, id: 456 } })), /endpoints disagree/)
  await assert.rejects(discoverRelease(repo, tag, fixture({ listing: [{ ...draft, draft: false }] })), /publication changed/)
})

test("invalid and changed immutable identities cannot be adopted", async () => {
  for (const value of [0, -1, "123", Number.MAX_SAFE_INTEGER + 1]) {
    await assert.rejects(discoverRelease(repo, tag, fixture({ listing: [{ ...draft, id: value }] })), /identity changed/)
  }
  for (const direct of [null, { ...draft, id: 456 }, { ...draft, tag_name: "untagged-abcd" }]) {
    await assert.rejects(discoverRelease(repo, tag, fixture({ direct })), /identity changed/)
  }
  await assert.rejects(discoverRelease(repo, tag, fixture({ byTag: { ...draft, draft: false, tag_name: "v0.8.0" } })), /identity changed/)
})

test("list-to-ID metadata and actor changes all fail closed", async () => {
  for (const patch of [{ draft: false }, { prerelease: true }, { target_commitish: "b".repeat(40) }, { name: "other" }, { body: body + "edited" }]) {
    await assert.rejects(discoverRelease(repo, tag, fixture({ direct: { ...draft, ...patch } })), /metadata changed/)
  }
  for (const patch of [{ id: 123 }, { login: "other" }, { type: "User" }]) {
    await assert.rejects(discoverRelease(repo, tag, fixture({ direct: { ...draft, author: { ...draft.author, ...patch } } })), /author changed/)
  }
})

test("API failures and malformed pages cannot masquerade as absence", async () => {
  const denied = new Error("HTTP 403")
  await assert.rejects(discoverRelease(repo, tag, { request: async () => { throw denied } }), error => error === denied)
  await assert.rejects(discoverRelease(repo, tag, fixture({ listing: {} })), /invalid GitHub API page/)
  const f = fixture()
  await assert.rejects(discoverRelease(repo, tag, { request: async (path, options) => {
    if (path === `${base}/123`) throw new Error("HTTP 404")
    return f.request(path, options)
  } }), /HTTP 404/)
})

test("discovery does not adopt detached aliases or bypass existing source-marker guards", async () => {
  assert.equal(await discoverRelease(repo, tag, fixture({ listing: [{ ...draft, tag_name: "untagged-abcd" }] })), null)
  for (const altered of [{ ...draft, body: "untrusted" }, { ...draft, target_commitish: "b".repeat(40) }]) {
    const found = await discoverRelease(repo, tag, fixture({ listing: [altered], direct: altered }))
    assert.throws(() => assertReleaseState(null, found, sha))
  }
})

test("canonical metadata assertions pin ID/tag/name/body through receipt and publication", () => {
  const expected = { id: 123, tag, name: draft.name, body }
  assertStagingMetadata(draft, expected)
  assertStagingMetadata({ ...draft, draft: false }, expected)
  for (const patch of [{ id: 456 }, { tag_name: "v0.8.0" }, { name: "other" }, { body: body + "\n" }]) {
    assert.throws(() => assertStagingMetadata({ ...draft, ...patch }, expected))
  }
})

test("canonical publisher wires discovery and every identity check without metadata repair or asset replacement", () => {
  const script = readFileSync(new URL("./release-publish.mjs", import.meta.url), "utf8")
  assert.match(script, /const release = await discoverRelease\(repo, tag\)/)
  assert.match(script, /const body = releaseBody\(readFileSync\("CHANGELOG.md", "utf8"\), version, sha\)/)
  assert.match(script, /body: current.body/)
  for (const name of ["release", "beforePublish", "publishSource.release", "beforeReceipt", "finalDraft", "published", "verified"]) {
    assert.ok(script.includes(`assertStagingMetadata(${name}, staging)`), name)
  }
  assert.equal((script.match(/method: "PATCH"/g) || []).length, 1)
  assert.match(script, /method: "PATCH", body: \{ draft: false, prerelease: false, make_latest: "true" \}/)
  assert.doesNotMatch(script, /method: "DELETE"/)
})
