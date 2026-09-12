import assert from "node:assert/strict"
import test from "node:test"
import {
  assertPublishedVersion,
  expectedReleaseAssetNames,
  publicationReceiptName,
  verifyPublicationReceipt,
} from "./release-lib.mjs"

const bot = { login: "github-actions[bot]", type: "Bot", id: 41898282 }
const version = "0.7.1"
const source = "a".repeat(40)
const manifestDigest = "b".repeat(64)
const receiptName = publicationReceiptName(version)

function releaseFixture() {
  const assets = expectedReleaseAssetNames(version).map((name, index) => ({
    name,
    state: "uploaded",
    digest: name === "SHA256SUMS" ? `sha256:${manifestDigest}` : `sha256:${String(index + 1).padStart(64, "c")}`.slice(0, 71),
    size: index + 1,
    uploader: bot,
    browser_download_url: `https://github.com/owner/repository/releases/download/v${version}/${name}`,
  }))
  return {
    tag_name: `v${version}`,
    target_commitish: source,
    body: `<!-- darkphish-release-source:${source} -->`,
    author: bot,
    assets,
    draft: false,
    prerelease: false,
    published_at: "2026-09-12T10:00:00Z",
  }
}

function receipt(overrides = {}) {
  return `${JSON.stringify({
    schema: "darkphish-release-publication-receipt/v1",
    tag: `v${version}`,
    source_sha: source,
    checksums_sha256: manifestDigest,
    ...overrides,
  })}\n`
}

test("v0.7.1 and later require a post-gate receipt while legacy v0.7.0 remains compatible", () => {
  assert.equal(publicationReceiptName("0.7.0"), null)
  assert.equal(receiptName, "darkphish-v0.7.1.release.json")
  assert.equal(expectedReleaseAssetNames("0.7.0").length, 7)
  assert.equal(expectedReleaseAssetNames(version).length, 8)
  assert.ok(expectedReleaseAssetNames(version).includes(receiptName))
  const release = releaseFixture()
  assert.equal(assertPublishedVersion(source, release, version), release)
  assert.throws(() => assertPublishedVersion(source, { ...release, assets: release.assets.filter((asset) => asset.name !== receiptName) }, version), /complete trusted artifact set/)
})

test("publication receipt proves exact source, tag and checksum manifest after final gates", async () => {
  const release = releaseFixture()
  const download = async (url) => {
    assert.equal(new URL(url).pathname.endsWith(`/${receiptName}`), true)
    return new Response(receipt(), { status: 200 })
  }
  assert.equal(await verifyPublicationReceipt(release, version, { download }), true)

  for (const [overrides, pattern] of [
    [{ source_sha: "d".repeat(40) }, /does not prove/],
    [{ tag: "v0.7.2" }, /does not prove/],
    [{ checksums_sha256: "e".repeat(64) }, /does not prove/],
    [{ schema: "untrusted/v1" }, /does not prove/],
    [{ extra: "field" }, /does not prove/],
  ]) {
    await assert.rejects(verifyPublicationReceipt(release, version, { download: async () => new Response(receipt(overrides), { status: 200 }) }), pattern)
  }
  await assert.rejects(verifyPublicationReceipt(release, version, { download: async () => new Response("not-json", { status: 200 }) }), /malformed/)
})

test("a human-published draft without the post-gate receipt cannot authorize the next patch", async () => {
  const release = releaseFixture()
  release.assets = release.assets.filter((asset) => asset.name !== receiptName)
  assert.throws(() => assertPublishedVersion(source, release, version), /complete trusted artifact set/)
  await assert.rejects(verifyPublicationReceipt(release, version, { download: async () => new Response(receipt(), { status: 200 }) }), /unavailable/)
})
