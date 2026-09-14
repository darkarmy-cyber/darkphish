import { createHash } from "node:crypto"
import { releaseBody } from "./release-notes.mjs"

const bot = (actor) => actor?.login === "github-actions[bot]" && actor?.type === "Bot" && actor?.id === 41898282
const semver = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/
const stableVersion = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/
const sha40 = (value) => /^[a-f0-9]{40}$/.test(value || "")

function versionTag(version) {
  if (!stableVersion.test(version || "")) throw new Error("release version must be stable SemVer")
  return `v${version}`
}

function versionAtLeast(version, floor) {
  versionTag(version); versionTag(floor)
  const a = version.split(".").map(Number), b = floor.split(".").map(Number)
  for (let i = 0; i < 3; i += 1) if (a[i] !== b[i]) return a[i] > b[i]
  return true
}

function publicationReceiptName(version) {
  return versionAtLeast(version, "0.7.1") ? `darkphish-v${version}.release.json` : null
}

function expectedReleaseAssetNames(version) {
  const tag = versionTag(version), receipt = publicationReceiptName(version)
  return [
    `darkphish-${tag}-darwin-amd64.tar.gz`,
    `darkphish-${tag}-darwin-arm64.tar.gz`,
    `darkphish-${tag}-linux-amd64.tar.gz`,
    `darkphish-${tag}-linux-arm64.tar.gz`,
    `darkphish-${tag}-windows-amd64.zip`,
    `darkphish-${tag}.spdx.json`,
    "SHA256SUMS",
    ...(receipt ? [receipt] : []),
  ].sort()
}

async function api(path, { method = "GET", body } = {}) {
  if (typeof path !== "string" || !/^repos\/[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+\//.test(path)) throw new Error("GitHub API path is invalid")
  const response = await fetch(`https://api.github.com/${path}`, {
    method,
    redirect: "error",
    signal: AbortSignal.timeout(15000),
    headers: {
      Accept: "application/vnd.github+json",
      Authorization: `Bearer ${process.env.GH_TOKEN || process.env.GITHUB_TOKEN}`,
      "X-GitHub-Api-Version": "2022-11-28",
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!response.ok) throw new Error(`GitHub ${method} request failed with HTTP ${response.status}`)
  return response.status === 204 ? null : response.json()
}

async function pages(path) {
  const all = []
  for (let page = 1; page <= 100; page += 1) {
    const rows = await api(`${path}${path.includes("?") ? "&" : "?"}per_page=100&page=${page}`)
    if (!Array.isArray(rows) || rows.length > 100) throw new Error("invalid GitHub API page")
    all.push(...rows)
    if (rows.length < 100) return all
  }
  throw new Error("GitHub pagination limit reached")
}

async function peelTagToCommit(ref, fetchTag) {
  let object = ref?.object
  const seen = new Set()
  for (let depth = 0; depth <= 8; depth += 1) {
    if (!object || !sha40(object.sha) || !["commit", "tag", "tree", "blob"].includes(object.type)) throw new Error("release tag has malformed Git object metadata")
    if (object.type === "commit") return object.sha
    if (object.type !== "tag") throw new Error("release tag must resolve to a commit")
    if (seen.has(object.sha) || depth === 8) throw new Error("release tag cannot be safely resolved")
    seen.add(object.sha)
    const tag = await fetchTag(object.sha)
    object = tag?.object
  }
  throw new Error("release tag does not resolve to a commit")
}

async function sourceText(repo, source) {
  const file = await api(`repos/${repo}/contents/CHANGELOG.md?ref=${source}`)
  if (file?.type !== "file" || file.encoding !== "base64" || typeof file.content !== "string") throw new Error("release source changelog is unavailable")
  return Buffer.from(file.content.replace(/\n/g, ""), "base64").toString("utf8")
}

async function tagCommit(repo, tag) {
  const ref = await api(`repos/${repo}/git/ref/tags/${tag}`)
  return peelTagToCommit(ref, (sha) => api(`repos/${repo}/git/tags/${sha}`))
}

function assertAssetMetadata(release, version) {
  const expected = expectedReleaseAssetNames(version)
  const assets = release.assets
  if (!Array.isArray(assets) || assets.length !== expected.length) throw new Error(`${release.tag_name}: release asset set is incomplete`)
  const names = assets.map((asset) => asset?.name).sort()
  if (new Set(names).size !== names.length || names.join("\n") !== expected.join("\n")) throw new Error(`${release.tag_name}: release asset names differ from the trusted set`)
  for (const asset of assets) {
    if (asset?.state !== "uploaded" || !bot(asset.uploader) || !/^sha256:[a-f0-9]{64}$/.test(asset?.digest || "") || !Number.isSafeInteger(asset?.size) || asset.size <= 0) {
      throw new Error(`${release.tag_name}: release asset metadata is untrusted`)
    }
  }
}

async function downloadAsset(repo, asset) {
  const expected = `https://api.github.com/repos/${repo}/releases/assets/${asset.id}`
  if (asset?.url !== expected || !Number.isSafeInteger(asset?.id) || asset.id < 1) throw new Error("release asset API identity is malformed")
  const response = await fetch(expected, {
    headers: {
      Accept: "application/octet-stream",
      Authorization: `Bearer ${process.env.GH_TOKEN || process.env.GITHUB_TOKEN}`,
      "X-GitHub-Api-Version": "2022-11-28",
    },
    redirect: "follow",
    signal: AbortSignal.timeout(30000),
  })
  if (!response.ok) throw new Error(`${asset.name}: release asset download failed with HTTP ${response.status}`)
  const bytes = Buffer.from(await response.arrayBuffer())
  const digest = createHash("sha256").update(bytes).digest("hex")
  if (asset.digest !== `sha256:${digest}` || asset.size !== bytes.length) throw new Error(`${asset.name}: downloaded bytes differ from GitHub digest metadata`)
  return bytes
}

async function verifyArtifactProofs(repo, release, version) {
  assertAssetMetadata(release, version)
  const manifest = release.assets.find((asset) => asset.name === "SHA256SUMS")
  const manifestText = (await downloadAsset(repo, manifest)).toString("utf8")
  const expected = new Map()
  for (const line of manifestText.trim().split(/\r?\n/)) {
    const match = line.match(/^([a-f0-9]{64})  (darkphish-[A-Za-z0-9._-]+)$/)
    if (!match || expected.has(match[2])) throw new Error(`${release.tag_name}: checksum manifest is malformed`)
    expected.set(match[2], match[1])
  }
  const payloads = release.assets.filter((asset) => asset.name !== "SHA256SUMS" && !asset.name.endsWith(".release.json"))
  if (expected.size !== payloads.length) throw new Error(`${release.tag_name}: checksum manifest is incomplete`)
  for (const asset of payloads) if (expected.get(asset.name) !== asset.digest.replace(/^sha256:/, "")) throw new Error(`${release.tag_name}: checksum manifest does not bind ${asset.name}`)

  const receiptName = publicationReceiptName(version)
  if (!receiptName) return
  const receiptAsset = release.assets.find((asset) => asset.name === receiptName)
  let receipt
  try { receipt = JSON.parse((await downloadAsset(repo, receiptAsset)).toString("utf8")) } catch { throw new Error(`${release.tag_name}: publication receipt is malformed`) }
  const expectedReceipt = {
    schema: "darkphish-release-publication-receipt/v1",
    tag: release.tag_name,
    source_sha: release.target_commitish,
    checksums_sha256: manifest.digest.replace(/^sha256:/, ""),
  }
  if (Object.keys(receipt).sort().join("\n") !== Object.keys(expectedReceipt).sort().join("\n") || Object.entries(expectedReceipt).some(([key, value]) => receipt[key] !== value)) throw new Error(`${release.tag_name}: publication receipt does not bind the release source and manifest`)
}

async function verifyTrustedRelease(repo, summary) {
  const match = semver.exec(summary?.tag_name || "")
  if (!match || !Number.isSafeInteger(summary?.id) || summary.id < 1 || summary.prerelease !== false || !bot(summary.author) || !sha40(summary.target_commitish)) return null
  const version = summary.tag_name.slice(1)
  if (versionTag(version) !== summary.tag_name) throw new Error("release tag canonicalization failed")
  const source = summary.target_commitish
  if (await tagCommit(repo, summary.tag_name) !== source) throw new Error(`${summary.tag_name}: immutable tag does not match release source`)
  const release = await api(`repos/${repo}/releases/${summary.id}`)
  if (release.tag_name !== summary.tag_name || release.target_commitish !== source || release.prerelease !== false || !bot(release.author)) throw new Error(`${summary.tag_name}: release identity changed during reconciliation`)
  await verifyArtifactProofs(repo, release, version)
  const changelog = await sourceText(repo, source)
  const body = releaseBody(changelog, version, source)
  const name = `Darkphish ${version.split(".").slice(0, 2).join(".")}`
  return { release, version, source, body, name }
}

async function reconcile(repo, summary) {
  const trusted = await verifyTrustedRelease(repo, summary)
  if (!trusted) return false
  const { release, body, name } = trusted
  if (![true, false].includes(release.draft)) throw new Error(`${release.tag_name}: release has an invalid draft state`)
  if (release.body === body && release.name === name) return false

  const patched = await api(`repos/${repo}/releases/${release.id}`, { method: "PATCH", body: {
    name,
    body,
    draft: release.draft,
    prerelease: false,
    make_latest: release.draft ? "legacy" : (release.tag_name === "v0.7.0" ? "true" : "legacy"),
  } })
  if (patched.id !== release.id || patched.tag_name !== release.tag_name || patched.target_commitish !== release.target_commitish || patched.draft !== release.draft || patched.prerelease !== false || patched.body !== body || patched.name !== name || !bot(patched.author)) throw new Error(`${release.tag_name}: reconciled release metadata failed verification`)
  await verifyArtifactProofs(repo, patched, trusted.version)
  console.log(`${release.tag_name}: notes reconciled${release.draft ? " (draft preserved)" : ""}`)
  return true
}

async function run() {
  const repo = process.env.GITHUB_REPOSITORY || process.env.GH_REPO
  if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repo || "")) throw new Error("GITHUB_REPOSITORY is required")
  const metadata = await api(`repos/${repo}`)
  const main = await api(`repos/${repo}/branches/main`)
  if (metadata.default_branch !== "main" || metadata.private !== false || metadata.fork !== false || main.protected !== true) throw new Error("release reconciliation requires standalone public protected main")
  const releases = await pages(`repos/${repo}/releases`)
  let changed = 0
  for (const release of releases.sort((a, b) => b.id - a.id)) if (await reconcile(repo, release)) changed += 1
  console.log(`Release reconciliation complete; ${changed} release(s) updated.`)
}

run().catch((error) => { console.error(error.message); process.exitCode = 1 })
