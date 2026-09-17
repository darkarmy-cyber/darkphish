import { appendFileSync, readFileSync, readdirSync, statSync } from "node:fs"
import { createHash } from "node:crypto"
import { api, assetDisposition, assertReleaseState, generatedPath, git, greenCommit, pages, protectedMain, publicationReceiptName, repository, verifyChecksums, versionTag } from "./release-lib.mjs"
import { verifyCodeQLBaseline } from "./codeql-baseline.mjs"
import { verifyReleaseMaintainerReview } from "./release-maintainer-review.mjs"
import { discoverRelease, assertStagingMetadata } from "./release-discovery.mjs"
import { releaseBody } from "./release-notes.mjs"
import { uploadReleaseAsset } from "./release-upload.mjs"

const trustedActionsActor = (actor) => actor?.login === "github-actions[bot]" && actor?.type === "Bot" && actor?.id === 41898282

function assertTrustedDraftRelease(release, sha) {
  if (!release || release.draft !== true || release.prerelease !== false || release.target_commitish !== sha || !trustedActionsActor(release.author)) {
    throw new Error("release staging draft must be owned by the immutable GitHub Actions actor for the exact source commit")
  }
}
function assertTrustedPublishedRelease(release, sha) {
  if (!release || release.draft !== false || release.prerelease !== false || release.target_commitish !== sha || !trustedActionsActor(release.author)) {
    throw new Error("release publication must be owned by the immutable GitHub Actions actor for the exact source commit")
  }
}
function assertTrustedReleaseAsset(asset) {
  if (asset?.state !== "uploaded" || !trustedActionsActor(asset?.uploader)) throw new Error("release contains an asset not uploaded by the immutable GitHub Actions actor")
}
function assertExactAssetSet(assets, names, hashes, bytes, receiptName, receiptHash, receiptLength) {
  const expectedNames = [...names, receiptName].sort()
  if (!Array.isArray(assets) || assets.length !== expectedNames.length) throw new Error("final release asset set is incomplete or unexpected")
  const actualNames = assets.map((asset) => asset?.name)
  if (new Set(actualNames).size !== actualNames.length || actualNames.slice().sort().join("\n") !== expectedNames.join("\n")) throw new Error("final release asset names changed after verification")
  for (const asset of assets) {
    assertTrustedReleaseAsset(asset)
    if (asset.name === receiptName) assetDisposition(asset, receiptHash, receiptLength)
    else assetDisposition(asset, hashes.get(asset.name), bytes.get(asset.name)?.length)
  }
  return true
}
function parseFragmentVersion(path) {
  const body = readFileSync(path, "utf8").replace(/\r\n/g, "\n")
  const match = body.match(/^---\ncategory: [A-Za-z]+\nversion: (\d+\.\d+\.\d+)\n---\n/)
  if (!match) throw new Error(`invalid changelog fragment schema: ${path}`)
  return match[1]
}
function compareVersions(a, b) {
  const left = a.split(".").map(Number), right = b.split(".").map(Number)
  for (let i = 0; i < 3; i += 1) if (left[i] !== right[i]) return left[i] - right[i]
  return 0
}
function verifyRetainedFragments(releaseVersion) {
  for (const name of readdirSync("changes")) {
    if (!name.endsWith(".md") || name === "README.md") continue
    const fragmentVersion = parseFragmentVersion(`changes/${name}`)
    if (compareVersions(fragmentVersion, releaseVersion) <= 0) throw new Error(`release source contains unconsumed fragment ${name} targeting ${fragmentVersion}`)
  }
}
async function uploadAsset(release, name, content) {
  return uploadReleaseAsset(repository(), release, name, content)
}

async function source() {
  const repo = repository(), sha = git("rev-parse", "HEAD"), version = readFileSync("VERSION", "utf8").trim(), tag = versionTag(version)
  await protectedMain(repo, sha)
  if (!await greenCommit(repo, sha)) return null
  const prs = await pages(`repos/${repo}/commits/${sha}/pulls`)
  const pr = prs.find((item) => item.merged_at && item.merge_commit_sha === sha && item.base.ref === "main" && item.head.ref === `release/${tag}` && item.title === `release: Darkphish ${version}`)
  if (!pr) return null
  await verifyReleaseMaintainerReview(repo, await api(`repos/${repo}/pulls/${pr.number}`), { get: api })
  await verifyCodeQLBaseline(repo, sha)
  const files = await pages(`repos/${repo}/pulls/${pr.number}/files`)
  if (!files.length || files.some((file) => !generatedPath(file.filename))) throw new Error("release PR includes application changes")
  verifyRetainedFragments(version)
  const body = releaseBody(readFileSync("CHANGELOG.md", "utf8"), version, sha)
  const name = `Darkphish ${version.split(".").slice(0, 2).join(".")}`
  let ref = await api(`repos/${repo}/git/ref/tags/${tag}`, { missing: true })
  if (ref?.object.type === "tag") ref = await api(`repos/${repo}/git/tags/${ref.object.sha}`)
  const release = await discoverRelease(repo, tag)
  const state = assertReleaseState(ref?.object.sha, release, sha)
  if (release?.draft) {
    assertTrustedDraftRelease(release, sha)
    assertStagingMetadata(release, { tag, name, body })
  }
  return { repo, sha, version, tag, release, state, name, body, builtAt: git("show", "-s", "--format=%cI", sha) }
}

async function run() {
  const command = process.argv[2], current = await source()
  if (command === "metadata") {
    const ready = current && current.state !== "published"
    const output = ready ? `ready=true\nversion=${current.version}\ntag=${current.tag}\nsha=${current.sha}\nbuilt_at=${new Date(current.builtAt).toISOString()}\n` : "ready=false\n"
    if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, output)
    console.log(ready ? `Verified protected release source ${current.sha}` : "No unpublished, fully checked release merge is ready.")
    return
  }
  if (command !== "publish" || !current) throw new Error("publication requires the fully checked protected release merge")
  const { repo, sha, version, tag } = current
  const receiptName = publicationReceiptName(version)
  if (!receiptName) throw new Error("publication receipt is required for this release line")
  const expected = ["linux-amd64.tar.gz", "linux-arm64.tar.gz", "windows-amd64.zip", "darwin-amd64.tar.gz", "darwin-arm64.tar.gz"].map((target) => `darkphish-${tag}-${target}`)
  expected.push(`darkphish-${tag}.spdx.json`, "SHA256SUMS")
  const names = readdirSync("dist").filter((name) => statSync(`dist/${name}`).isFile()).sort()
  if (names.join("\n") !== expected.sort().join("\n")) throw new Error("native release artifact set is incomplete or unexpected")
  const bytes = new Map(names.map((name) => [name, readFileSync(`dist/${name}`)]))
  const hashes = new Map([...bytes].map(([name, value]) => [name, createHash("sha256").update(value).digest("hex")]))
  verifyChecksums(readFileSync("dist/SHA256SUMS", "utf8"), hashes)

  await protectedMain(repo, sha)
  await verifyCodeQLBaseline(repo, sha)
  const finalSource = await source()
  if (!finalSource || finalSource.sha !== sha || finalSource.tag !== tag || finalSource.state === "published") throw new Error("release source or review readiness changed before staging")

  let release = finalSource.release
  if (!release) {
    release = await api(`repos/${repo}/releases`, { method: "POST", body: {
      tag_name: tag, target_commitish: sha, name: current.name,
      draft: true, prerelease: false, make_latest: "true",
      body: current.body,
    } })
  }
  assertTrustedDraftRelease(release, sha)
  const staging = { id: release.id, tag, name: current.name, body: current.body }
  assertStagingMetadata(release, staging)

  const assets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  const allowedNames = new Set([...names, receiptName])
  if (assets.some((asset) => !allowedNames.has(asset.name)) || new Set(assets.map((asset) => asset.name)).size !== assets.length) throw new Error("release draft contains unexpected assets")
  for (const asset of assets) assertTrustedReleaseAsset(asset)
  for (const name of names) {
    const content = bytes.get(name)
    if (assetDisposition(assets.find((asset) => asset.name === name), hashes.get(name), content.length) === "reuse") continue
    await uploadAsset(release, name, content)
  }

  const uploaded = await pages(`repos/${repo}/releases/${release.id}/assets`)
  for (const name of names) {
    const asset = uploaded.find((candidate) => candidate.name === name)
    if (!asset) throw new Error("release asset missing after upload")
    assertTrustedReleaseAsset(asset)
    assetDisposition(asset, hashes.get(name), bytes.get(name).length)
  }

  await protectedMain(repo, sha)
  await verifyCodeQLBaseline(repo, sha)
  const beforePublish = await api(`repos/${repo}/releases/${release.id}`)
  assertTrustedDraftRelease(beforePublish, sha)
  assertStagingMetadata(beforePublish, staging)
  const publishSource = await source()
  if (!publishSource || publishSource.sha !== sha || publishSource.tag !== tag || publishSource.state !== "resume") throw new Error("release source changed before final publication")
  assertStagingMetadata(publishSource.release, staging)

  const receipt = Buffer.from(`${JSON.stringify({
    schema: "darkphish-release-publication-receipt/v1",
    tag,
    source_sha: sha,
    checksums_sha256: hashes.get("SHA256SUMS"),
  })}\n`, "utf8")
  const receiptHash = createHash("sha256").update(receipt).digest("hex")
  const beforeReceipt = await api(`repos/${repo}/releases/${release.id}`)
  assertTrustedDraftRelease(beforeReceipt, sha)
  assertStagingMetadata(beforeReceipt, staging)
  let receiptAssets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  const existingReceipt = receiptAssets.find((asset) => asset.name === receiptName)
  if (assetDisposition(existingReceipt, receiptHash, receipt.length) === "upload") await uploadAsset(release, receiptName, receipt)

  receiptAssets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  assertExactAssetSet(receiptAssets, names, hashes, bytes, receiptName, receiptHash, receipt.length)

  const finalDraft = await api(`repos/${repo}/releases/${release.id}`)
  assertTrustedDraftRelease(finalDraft, sha)
  assertStagingMetadata(finalDraft, staging)
  const finalAssets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  assertExactAssetSet(finalAssets, names, hashes, bytes, receiptName, receiptHash, receipt.length)

  const published = await api(`repos/${repo}/releases/${release.id}`, { method: "PATCH", body: { draft: false, prerelease: false, make_latest: "true" } })
  assertTrustedPublishedRelease(published, sha)
  assertStagingMetadata(published, staging)
  const verified = await api(`repos/${repo}/releases/${release.id}`)
  assertTrustedPublishedRelease(verified, sha)
  assertStagingMetadata(verified, staging)
  const publishedAssets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  assertExactAssetSet(publishedAssets, names, hashes, bytes, receiptName, receiptHash, receipt.length)
  console.log(`Published and verified https://github.com/${repo}/releases/tag/${tag}`)
}
run().catch((error) => { console.error(error.message); process.exitCode = 1 })
