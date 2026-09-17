import assert from "node:assert/strict"
import { createHash } from "node:crypto"
import { readFileSync } from "node:fs"
import test from "node:test"
import { uploadReleaseAsset, uploadTimeoutMs } from "./release-upload.mjs"

const repo = "owner/repo", name = "darkphish-v0.13.0-linux-amd64.tar.gz", content = Buffer.from("verified archive")
const release = { id: 42, draft: true, upload_url: `https://uploads.github.com/repos/${repo}/releases/42/assets{?name,label}` }
const asset = { id: 71, name, state: "uploaded", size: content.length, digest: `sha256:${createHash("sha256").update(content).digest("hex")}`, uploader: { id: 41898282, login: "github-actions[bot]", type: "Bot" } }
function fixture({ status = 201, transport = false, rows = [asset], failRead = false, cancelFails = false } = {}) {
  const seen = { posts: 0, reads: 0, cancels: 0, pauses: 0 }
  const options = {
    token: "unit-test-placeholder",
    request: async (url, init) => {
      seen.posts += 1
      assert.equal(url.href, `https://uploads.github.com/repos/${repo}/releases/42/assets?name=${name}`)
      assert.equal(init.method, "POST"); assert.equal(init.redirect, "error")
      assert.equal(init.headers["Content-Length"], String(content.length)); assert.equal(init.body, content)
      assert.ok(init.signal instanceof AbortSignal)
      if (transport) throw new Error("private transport detail must not escape")
      return { status, body: { cancel: async () => { seen.cancels += 1; if (cancelFails) throw new Error("cancel failed") } } }
    },
    list: async (path) => {
      assert.equal(path, `repos/${repo}/releases/42/assets`); seen.reads += 1
      if (failRead) throw new Error("read unavailable")
      return typeof rows === "function" ? rows(seen.reads) : rows
    },
    pause: async (ms) => { assert.equal(ms, 2000); seen.pauses += 1 },
  }
  return { seen, options, run: () => uploadReleaseAsset(repo, release, name, content, options) }
}

test("upload deadline covers observed 118-second upload but remains bounded", () => {
  assert.equal(uploadTimeoutMs, 180000)
})
for (const scenario of [{ status: 201 }, { status: 500 }, { status: 502 }, { transport: true }, { cancelFails: true }]) {
  test(`exact trusted read-back confirms one upload ${JSON.stringify(scenario)}`, async () => {
    const f = fixture(scenario)
    assert.deepEqual(await f.run(), asset)
    assert.equal(f.seen.posts, 1); assert.equal(f.seen.reads, 1)
    assert.equal(f.seen.cancels, scenario.transport ? 0 : 1)
  })
}
test("collection visibility may converge through bounded reads without replay", async () => {
  const f = fixture({ status: 500, rows: (read) => read === 3 ? [asset] : [] })
  assert.deepEqual(await f.run(), asset)
  assert.deepEqual(f.seen, { posts: 1, reads: 3, cancels: 1, pauses: 2 })
})
for (const scenario of [{ status: 201 }, { status: 500 }, { transport: true }]) {
  test(`missing asset fails closed after three reads ${JSON.stringify(scenario)}`, async () => {
    const f = fixture({ ...scenario, rows: [] })
    await assert.rejects(f.run, /not confirmed.*draft retained without replay/)
    assert.equal(f.seen.posts, 1); assert.equal(f.seen.reads, 3)
  })
}
for (const status of [200, 202, 301, 401, 403, 404, 422, 429]) {
  test(`HTTP ${status} is not converted to success or retried`, async () => {
    const f = fixture({ status })
    await assert.rejects(f.run, /rejected with HTTP/)
    assert.deepEqual(f.seen, { posts: 1, reads: 0, cancels: 1, pauses: 0 })
  })
}
test("wrong digest, length, identity, completion, duplicate and malformed listing fail closed", async () => {
  for (const rows of [
    [{ ...asset, digest: `sha256:${"0".repeat(64)}` }], [{ ...asset, digest: null }],
    [{ ...asset, size: content.length + 1 }], [{ ...asset, state: "starter" }],
    [{ ...asset, id: 0 }], [{ ...asset, uploader: { ...asset.uploader, id: 1 } }],
    [{ ...asset, uploader: { ...asset.uploader, login: "other" } }],
    [{ ...asset, uploader: { ...asset.uploader, type: "User" } }],
    [asset, asset], null,
  ]) {
    const f = fixture({ status: 500, rows })
    await assert.rejects(f.run)
    assert.equal(f.seen.posts, 1); assert.equal(f.seen.reads, 1)
  }
})
test("read failure cannot certify an ambiguous write", async () => {
  const f = fixture({ transport: true, failRead: true })
  await assert.rejects(f.run, /read unavailable/)
  assert.equal(f.seen.posts, 1)
})
test("destination and input checks happen before any credential-bearing request", async () => {
  const f = fixture()
  for (const changed of [
    { draft: false }, { id: -1 },
    { upload_url: "https://attacker.invalid/repos/owner/repo/releases/42/assets" },
    { upload_url: "https://uploads.github.com/repos/owner/other/releases/42/assets" },
    { upload_url: "https://uploads.github.com/repos/owner/repo/releases/43/assets" },
    { upload_url: "https://uploads.github.com/repos/owner/repo/releases/42/assets?other=true" },
    { upload_url: "https://user@uploads.github.com/repos/owner/repo/releases/42/assets" },
  ]) await assert.rejects(() => uploadReleaseAsset(repo, { ...release, ...changed }, name, content, f.options))
  await assert.rejects(() => uploadReleaseAsset("../repo", release, name, content, f.options))
  await assert.rejects(() => uploadReleaseAsset(repo, release, "../asset", content, f.options))
  await assert.rejects(() => uploadReleaseAsset(repo, release, name, "not bytes", f.options))
  await assert.rejects(() => uploadReleaseAsset(repo, release, name, content, { ...f.options, token: "" }))
  assert.equal(f.seen.posts, 0)
})
test("native and recovery publishers share the one-POST verified uploader", () => {
  for (const path of ["./release-publish.mjs", "../.github/scripts/release-recover.mjs"]) {
    const source = readFileSync(new URL(path, import.meta.url), "utf8")
    assert.match(source, /return uploadReleaseAsset\(repository\(\), release, name, content\)/)
  }
})
