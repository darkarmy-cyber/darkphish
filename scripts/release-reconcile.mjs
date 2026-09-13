import { api, expectedReleaseAssetNames, pages, peelTagToCommit, verifyPublicationReceipt, verifyPublishedAssetManifest, versionTag } from "./release-lib.mjs"
import { releaseBody } from "./release-notes.mjs"

const bot = (actor) => actor?.login === "github-actions[bot]" && actor?.type === "Bot" && actor?.id === 41898282
const semver = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/
const sha40 = (value) => /^[a-f0-9]{40}$/.test(value || "")

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

async function verifyTrustedRelease(repo, summary) {
  const match = semver.exec(summary?.tag_name || "")
  if (!match || !Number.isSafeInteger(summary?.id) || summary.id < 1 || summary.prerelease !== false || !bot(summary.author) || !sha40(summary.target_commitish)) return null
  const version = summary.tag_name.slice(1)
  if (versionTag(version) !== summary.tag_name) throw new Error("release tag canonicalization failed")
  const source = summary.target_commitish
  if (await tagCommit(repo, summary.tag_name) !== source) throw new Error(`${summary.tag_name}: immutable tag does not match release source`)
  const release = await api(`repos/${repo}/releases/${summary.id}`)
  if (release.tag_name !== summary.tag_name || release.target_commitish !== source || release.prerelease !== false || !bot(release.author)) throw new Error(`${summary.tag_name}: release identity changed during reconciliation`)
  assertAssetMetadata(release, version)
  await verifyPublishedAssetManifest(release)
  await verifyPublicationReceipt(release, version)
  const changelog = await sourceText(repo, source)
  const body = releaseBody(changelog, version, source)
  const name = `Darkphish ${version.split(".").slice(0, 2).join(".")}`
  return { release, version, source, body, name }
}

async function reconcile(repo, summary) {
  const trusted = await verifyTrustedRelease(repo, summary)
  if (!trusted) return false
  const { release, body, name } = trusted
  const republish = release.draft === true
  if (republish && !release.published_at) throw new Error(`${release.tag_name}: unpublished draft cannot be promoted by metadata reconciliation`)
  if (!republish && release.draft !== false) throw new Error(`${release.tag_name}: release has an invalid draft state`)
  if (!republish && release.body === body && release.name === name) return false

  const patched = await api(`repos/${repo}/releases/${release.id}`, { method: "PATCH", body: {
    name,
    body,
    draft: false,
    prerelease: false,
    make_latest: release.tag_name === "v0.7.1" ? "true" : "legacy",
  } })
  if (patched.id !== release.id || patched.tag_name !== release.tag_name || patched.target_commitish !== release.target_commitish || patched.draft !== false || patched.prerelease !== false || patched.body !== body || patched.name !== name || !bot(patched.author)) throw new Error(`${release.tag_name}: reconciled release metadata failed verification`)
  assertAssetMetadata(patched, trusted.version)
  await verifyPublishedAssetManifest(patched)
  await verifyPublicationReceipt(patched, trusted.version)
  console.log(`${release.tag_name}: ${republish ? "republished" : "notes reconciled"}`)
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
  for (const release of releases.sort((a, b) => a.id - b.id)) if (await reconcile(repo, release)) changed += 1
  console.log(`Release reconciliation complete; ${changed} release(s) updated.`)
}

run().catch((error) => { console.error(error.message); process.exitCode = 1 })
