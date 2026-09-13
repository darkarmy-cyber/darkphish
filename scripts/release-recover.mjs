import { createHash } from "node:crypto"
import { readFileSync } from "node:fs"
import {
  api, assetDisposition, expectedReleaseAssetNames, generatedPath, greenCommit, pages,
  publicationReceiptName, repository, versionTag,
} from "./release-lib.mjs"
import { verifyCodeQLBaseline } from "./codeql-baseline.mjs"
import { verifyReleaseMaintainerReview } from "./release-maintainer-review.mjs"

const actionsBot = (actor) => actor?.login === "github-actions[bot]" && actor?.type === "Bot" && actor?.id === 41898282
const sha40 = (value) => typeof value === "string" && /^[a-f0-9]{40}$/.test(value)

function assertTrustedDraft(release, version) {
  const tag = versionTag(version)
  if (!release || release.tag_name !== tag || release.draft !== true || release.prerelease !== false || !actionsBot(release.author)) {
    throw new Error("pending release is not a trusted GitHub Actions draft")
  }
  if (!sha40(release.target_commitish) || !release.body?.includes(`<!-- darkphish-release-source:${release.target_commitish} -->`)) {
    throw new Error("pending release has invalid source provenance")
  }
  return release
}

async function currentProtectedMain(repo) {
  const metadata = await api(`repos/${repo}`)
  const branch = await api(`repos/${repo}/branches/main`)
  if (metadata.default_branch !== "main" || metadata.private !== false || metadata.fork !== false || !branch.protected || !sha40(branch.commit?.sha)) {
    throw new Error("expected standalone public repository with protected main")
  }
  if (!await greenCommit(repo, branch.commit.sha)) throw new Error("current protected main is not green")
  return branch.commit.sha
}

async function verifySourceAncestry(repo, source, main) {
  if (source === main) return
  const comparison = await api(`repos/${repo}/compare/${source}...${main}`)
  if (comparison?.base_commit?.sha !== source || !["ahead", "identical"].includes(comparison?.status) || comparison.behind_by !== 0) {
    throw new Error("pending release source is not an ancestor of current protected main")
  }
}

async function verifiedReleasePR(repo, source, version, tag) {
  if (!await greenCommit(repo, source)) throw new Error("pending release source required checks are not green")
  const prs = await pages(`repos/${repo}/commits/${source}/pulls`)
  const matches = prs.filter((pr) => pr.merged_at && pr.merge_commit_sha === source && pr.base?.ref === "main" && pr.head?.ref === `release/${tag}` && pr.title === `release: Darkphish ${version}`)
  if (matches.length !== 1) throw new Error("pending release source does not map to exactly one trusted release PR")
  const pr = await api(`repos/${repo}/pulls/${matches[0].number}`)
  await verifyReleaseMaintainerReview(repo, pr, { get: api })
  await verifyCodeQLBaseline(repo, source)
  const files = await pages(`repos/${repo}/pulls/${pr.number}/files`)
  if (!files.length || files.some((file) => !generatedPath(file.filename))) throw new Error("pending release PR includes application changes")
  return pr
}

async function downloadAsset(asset) {
  if (!asset?.url || asset.state !== "uploaded" || !actionsBot(asset.uploader) || !/^sha256:[a-f0-9]{64}$/.test(asset.digest || "") || !Number.isSafeInteger(asset.size) || asset.size <= 0) {
    throw new Error("pending release contains an untrusted asset")
  }
  const response = await fetch(asset.url, {
    headers: {
      Accept: "application/octet-stream",
      Authorization: `Bearer ${process.env.GH_TOKEN || process.env.GITHUB_TOKEN}`,
      "X-GitHub-Api-Version": "2022-11-28",
    },
    redirect: "follow",
    signal: AbortSignal.timeout(30000),
  })
  if (!response.ok) throw new Error(`pending release asset download failed with HTTP ${response.status}`)
  const bytes = Buffer.from(await response.arrayBuffer())
  if (bytes.length !== asset.size || createHash("sha256").update(bytes).digest("hex") !== asset.digest.slice(7)) {
    throw new Error("pending release asset bytes do not match GitHub digest metadata")
  }
  return bytes
}

function parseManifest(text) {
  const entries = new Map()
  for (const line of text.trim().split(/\r?\n/)) {
    const match = line.match(/^([a-f0-9]{64})  (darkphish-[A-Za-z0-9._-]+)$/)
    if (!match || entries.has(match[2])) throw new Error("pending release checksum manifest is malformed")
    entries.set(match[2], match[1])
  }
  return entries
}

async function uploadAsset(release, name, bytes) {
  const url = new URL(release.upload_url.split("{")[0])
  if (url.origin !== "https://uploads.github.com") throw new Error("unexpected release upload host")
  url.searchParams.set("name", name)
  const response = await fetch(url, {
    method: "POST",
    headers: { Authorization: `Bearer ${process.env.GH_TOKEN || process.env.GITHUB_TOKEN}`, "Content-Type": "application/octet-stream" },
    body: bytes,
    redirect: "error",
    signal: AbortSignal.timeout(30000),
  })
  if (!response.ok) throw new Error(`release recovery asset upload failed with HTTP ${response.status}`)
}

async function recover() {
  const repo = repository()
  const version = readFileSync("VERSION", "utf8").trim()
  const tag = versionTag(version)
  const main = await currentProtectedMain(repo)
  const releases = await pages(`repos/${repo}/releases`)
  const drafts = releases.filter((release) => release?.draft === true && release?.tag_name === tag)
  if (drafts.length === 0) {
    console.log(`No pending ${tag} draft release requires recovery.`)
    return
  }
  if (drafts.length !== 1) throw new Error(`multiple pending ${tag} drafts require manual investigation`)

  let release = assertTrustedDraft(drafts[0], version)
  const source = release.target_commitish
  await verifySourceAncestry(repo, source, main)
  await verifiedReleasePR(repo, source, version, tag)

  const receiptName = publicationReceiptName(version)
  if (!receiptName) throw new Error("release recovery requires publication receipt support")
  const requiredWithoutReceipt = expectedReleaseAssetNames(version).filter((name) => name !== receiptName).sort()
  let assets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  const names = assets.map((asset) => asset.name)
  if (new Set(names).size !== names.length || names.filter((name) => name !== receiptName).sort().join("\n") !== requiredWithoutReceipt.join("\n")) {
    throw new Error("pending release asset set is incomplete or unexpected")
  }

  const bytes = new Map()
  for (const asset of assets.filter((asset) => asset.name !== receiptName)) bytes.set(asset.name, await downloadAsset(asset))
  const manifest = parseManifest(bytes.get("SHA256SUMS").toString("utf8"))
  const payloads = requiredWithoutReceipt.filter((name) => name !== "SHA256SUMS")
  if (manifest.size !== payloads.length) throw new Error("pending release checksum manifest is incomplete")
  for (const name of payloads) {
    const digest = createHash("sha256").update(bytes.get(name)).digest("hex")
    if (manifest.get(name) !== digest) throw new Error("pending release checksum manifest does not match uploaded assets")
  }

  const receipt = Buffer.from(`${JSON.stringify({
    schema: "darkphish-release-publication-receipt/v1",
    tag,
    source_sha: source,
    checksums_sha256: createHash("sha256").update(bytes.get("SHA256SUMS")).digest("hex"),
  })}\n`, "utf8")
  const receiptDigest = createHash("sha256").update(receipt).digest("hex")
  const existingReceipt = assets.find((asset) => asset.name === receiptName)
  if (assetDisposition(existingReceipt, receiptDigest, receipt.length) === "upload") await uploadAsset(release, receiptName, receipt)

  release = assertTrustedDraft(await api(`repos/${repo}/releases/${release.id}`), version)
  assets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  const expected = expectedReleaseAssetNames(version).sort()
  if (assets.length !== expected.length || assets.map((asset) => asset.name).sort().join("\n") !== expected.join("\n")) throw new Error("pending release final asset set is incomplete")
  for (const asset of assets) {
    if (asset.name === receiptName) {
      if (assetDisposition(asset, receiptDigest, receipt.length) !== "reuse") throw new Error("publication receipt changed during recovery")
    } else await downloadAsset(asset)
  }

  const finalMain = await currentProtectedMain(repo)
  if (finalMain !== main) throw new Error("protected main changed during release recovery")
  await verifySourceAncestry(repo, source, finalMain)
  await verifiedReleasePR(repo, source, version, tag)
  const published = await api(`repos/${repo}/releases/${release.id}`, { method: "PATCH", body: { draft: false, prerelease: false, make_latest: "true" } })
  if (published?.draft !== false || published?.tag_name !== tag || published?.target_commitish !== source || !actionsBot(published.author)) {
    throw new Error("release recovery publication verification failed")
  }
  console.log(`Recovered and published https://github.com/${repo}/releases/tag/${tag} from immutable source ${source}`)
}

recover().catch((error) => { console.error(error.message); process.exitCode = 1 })
