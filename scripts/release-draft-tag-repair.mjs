import { api, expectedReleaseAssetNames, pages, peelTagToCommit, versionTag } from "./release-lib.mjs"
import { releaseBody } from "./release-notes.mjs"

const bot = (actor) => actor?.login === "github-actions[bot]" && actor?.type === "Bot" && actor?.id === 41898282
const sha40 = (value) => typeof value === "string" && /^[a-f0-9]{40}$/.test(value)
const detachedTag = /^untagged-[a-f0-9]{20,64}$/
const canonicalName = /^Darkphish (0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/

async function sourceText(repo, source) {
  const file = await api(`repos/${repo}/contents/CHANGELOG.md?ref=${source}`)
  if (file?.type !== "file" || file.encoding !== "base64" || typeof file.content !== "string") throw new Error("detached draft source changelog is unavailable")
  return Buffer.from(file.content.replace(/\n/g, ""), "base64").toString("utf8")
}

function assetSnapshot(assets, version) {
  const expected = expectedReleaseAssetNames(version)
  if (!Array.isArray(assets) || assets.length !== expected.length) throw new Error("detached draft asset set is incomplete")
  const names = assets.map((asset) => asset?.name).sort()
  if (new Set(names).size !== names.length || names.join("\n") !== expected.join("\n")) throw new Error("detached draft asset names differ from the trusted set")
  const snapshot = new Map()
  for (const asset of assets) {
    if (!Number.isSafeInteger(asset?.id) || asset.id < 1 || asset?.state !== "uploaded" || !bot(asset.uploader) || !/^sha256:[a-f0-9]{64}$/.test(asset?.digest || "") || !Number.isSafeInteger(asset?.size) || asset.size <= 0) throw new Error("detached draft asset metadata is untrusted")
    snapshot.set(asset.name, { id: asset.id, url: asset.url, digest: asset.digest, size: asset.size, uploader: asset.uploader.id, state: asset.state })
  }
  return snapshot
}

function assertSameAssets(assets, snapshot) {
  if (assets.length !== snapshot.size) throw new Error("detached draft asset set changed during tag repair")
  for (const asset of assets) {
    const prior = snapshot.get(asset.name)
    if (!prior || asset.id !== prior.id || asset.url !== prior.url || asset.digest !== prior.digest || asset.size !== prior.size || asset.uploader?.id !== prior.uploader || asset.state !== prior.state) throw new Error("detached draft asset identity changed during tag repair")
  }
}

async function tagCommit(repo, tag) {
  const ref = await api(`repos/${repo}/git/ref/tags/${tag}`)
  if (!sha40(ref?.object?.sha) || !["commit", "tag"].includes(ref?.object?.type)) throw new Error(`${tag}: immutable tag metadata is malformed`)
  return peelTagToCommit(ref, (sha) => api(`repos/${repo}/git/tags/${sha}`))
}

async function repair(repo, summary, releases) {
  const match = canonicalName.exec(summary.name || "")
  if (!match || summary.draft !== true || summary.prerelease !== false || !detachedTag.test(summary.tag_name || "") || !Number.isSafeInteger(summary.id) || summary.id < 1 || !sha40(summary.target_commitish) || !bot(summary.author)) throw new Error("detached draft candidate identity is malformed")
  const version = `${match[1]}.${match[2]}.${match[3]}`
  const tag = versionTag(version)
  if (releases.some((release) => release.id !== summary.id && release.tag_name === tag)) throw new Error(`${tag}: canonical release identity already exists`)
  const source = summary.target_commitish
  if (await tagCommit(repo, tag) !== source) throw new Error(`${tag}: immutable tag does not match detached draft source`)

  const release = await api(`repos/${repo}/releases/${summary.id}`)
  if (release.id !== summary.id || release.tag_name !== summary.tag_name || release.target_commitish !== source || release.name !== summary.name || release.draft !== true || release.prerelease !== false || !bot(release.author)) throw new Error(`${tag}: detached draft changed before repair`)
  if (typeof release.published_at !== "string" || !Number.isFinite(Date.parse(release.published_at))) throw new Error(`${tag}: detached draft is not a historical rollback state`)

  const changelog = await sourceText(repo, source)
  const expectedBody = releaseBody(changelog, version, source)
  if (release.body !== expectedBody) throw new Error(`${tag}: detached draft body is not canonical source-derived metadata`)

  const assets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  const snapshot = assetSnapshot(assets, version)

  const patched = await api(`repos/${repo}/releases/${release.id}`, {
    method: "PATCH",
    body: {
      tag_name: tag,
      target_commitish: source,
      name: release.name,
      body: release.body,
      draft: true,
      prerelease: false,
    },
  })
  if (patched.id !== release.id || patched.tag_name !== tag || patched.target_commitish !== source || patched.name !== release.name || patched.body !== release.body || patched.draft !== true || patched.prerelease !== false || !bot(patched.author)) throw new Error(`${tag}: detached draft tag repair failed immediate verification`)
  if (await tagCommit(repo, tag) !== source) throw new Error(`${tag}: immutable tag changed during detached draft repair`)

  const closing = await api(`repos/${repo}/releases/${release.id}`)
  if (closing.id !== release.id || closing.tag_name !== tag || closing.target_commitish !== source || closing.name !== release.name || closing.body !== release.body || closing.draft !== true || closing.prerelease !== false || !bot(closing.author)) throw new Error(`${tag}: repaired draft changed during closing verification`)
  const closingAssets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  assertSameAssets(closingAssets, snapshot)
  assetSnapshot(closingAssets, version)
  console.log(`${tag}: restored trusted detached draft tag without changing source, body, state, title or assets.`)
}

async function run() {
  const repo = process.env.GITHUB_REPOSITORY || process.env.GH_REPO
  if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repo || "")) throw new Error("GITHUB_REPOSITORY is required")
  const releases = await pages(`repos/${repo}/releases`)
  const candidates = releases.filter((release) => release?.draft === true && release?.prerelease === false && detachedTag.test(release?.tag_name || "") && canonicalName.test(release?.name || ""))
  if (candidates.length > 1) throw new Error("multiple detached canonical release drafts require manual forensic review")
  if (!candidates.length) {
    console.log("No detached canonical release draft requires repair.")
    return
  }
  await repair(repo, candidates[0], releases)
}

run().catch((error) => { console.error(error.message); process.exitCode = 1 })
