import { createHash } from "node:crypto"
import { execFileSync } from "node:child_process"
import { mkdtempSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { api, assertCurrentVersionPublished, expectedReleaseAssetNames, greenCommit, pages, peelTagToCommit, repository, versionTag } from "../../scripts/release-lib.mjs"
import { verifyCodeQLBaseline } from "../../scripts/codeql-baseline.mjs"

const sha40 = (value) => typeof value === "string" && /^[a-f0-9]{40}$/.test(value)
const actionsBot = (actor) => actor?.login === "github-actions[bot]" && actor?.type === "Bot" && actor?.id === 41898282
const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

function notesForVersion(changelog, version) {
  const start = changelog.indexOf(`## ${version} - `)
  if (start < 0) throw new Error("release source changelog is missing the release section")
  const end = changelog.indexOf("\n## ", start + 1)
  return changelog.slice(start, end < 0 ? undefined : end).trim()
}
function releaseName(version) { return `Darkphish ${version.split(".").slice(0, 2).join(".")}` }
function releaseBody(notes, source) { return `${notes}\n\nSource commit: ${source}\n\n<!-- darkphish-release-source:${source} -->\n\nNative binaries, SHA-256 checksums and SPDX SBOM are attached.` }
async function sourceText(repo, path, source) {
  const file = await api(`repos/${repo}/contents/${path}?ref=${source}`)
  if (file?.type !== "file" || file.encoding !== "base64" || typeof file.content !== "string") throw new Error(`release source ${path} is unavailable`)
  return Buffer.from(file.content.replace(/\n/g, ""), "base64").toString("utf8")
}
async function expectedMetadata(repo, source, version) {
  return { name: releaseName(version), body: releaseBody(notesForVersion(await sourceText(repo, "CHANGELOG.md", source), version), source) }
}
function assertExactPublished(release, version, source, expected) {
  if (!release || release.tag_name !== versionTag(version) || release.target_commitish !== source || release.name !== expected.name || release.body !== expected.body || release.draft !== false || release.prerelease !== false || !release.published_at || !actionsBot(release.author)) throw new Error("published recovery metadata changed before attestation acceptance")
  return release
}
async function readTagState(repo, tag) {
  const ref = await api(`repos/${repo}/git/ref/tags/${tag}`, { missing: true })
  if (!ref) return null
  const objectType = ref.object?.type, objectSha = ref.object?.sha
  if (!sha40(objectSha) || !["commit", "tag"].includes(objectType)) throw new Error("recovery tag ref object is malformed")
  const commit = await peelTagToCommit(ref, (sha) => api(`repos/${repo}/git/tags/${sha}`))
  return { objectType, objectSha, commit }
}
function sameTagState(actual, expected) { return Boolean(actual && expected && actual.objectType === expected.objectType && actual.objectSha === expected.objectSha && actual.commit === expected.commit) }
async function expectedTagState(source) {
  const present = process.env.RECOVERY_TAG_PRESENT
  if (present === "false") return { objectType: "commit", objectSha: source, commit: source }
  if (present !== "true") throw new Error("recovery tag snapshot input is invalid")
  const objectType = process.env.RECOVERY_TAG_OBJECT_TYPE, objectSha = process.env.RECOVERY_TAG_OBJECT_SHA
  if (!["commit", "tag"].includes(objectType) || !sha40(objectSha)) throw new Error("recovery tag object snapshot is invalid")
  return { objectType, objectSha, commit: source }
}
async function assertExecutionMain(repo, expected) {
  if (!sha40(expected)) throw new Error("recovery execution SHA is malformed")
  const metadata = await api(`repos/${repo}`), branch = await api(`repos/${repo}/branches/main`)
  if (metadata?.default_branch !== "main" || metadata?.private !== false || metadata?.fork !== false || branch?.protected !== true || branch?.commit?.sha !== expected) throw new Error("protected main does not equal immutable recovery execution SHA")
  if (!await greenCommit(repo, expected)) throw new Error("recovery execution SHA required checks are not green")
  await verifyCodeQLBaseline(repo, expected)
  if (!await greenCommit(repo, expected)) throw new Error("required checks changed while verifying recovery publication")
  const closing = await api(`repos/${repo}/branches/main`)
  if (!closing?.protected || closing.commit?.sha !== expected) throw new Error("protected main moved during recovery publication verification")
}
async function downloadAsset(repo, asset, directory) {
  const url = new URL(asset?.url || "")
  if (!Number.isSafeInteger(asset?.id) || asset.id < 1 || url.protocol !== "https:" || url.hostname !== "api.github.com" || url.pathname !== `/repos/${repo}/releases/assets/${asset.id}`) throw new Error("recovery release asset metadata is invalid")
  const response = await fetch(url, { method: "GET", headers: { Accept: "application/octet-stream", Authorization: `Bearer ${process.env.GH_TOKEN || process.env.GITHUB_TOKEN}`, "X-GitHub-Api-Version": "2022-11-28" }, redirect: "follow", signal: AbortSignal.timeout(30000) })
  if (!response.ok) throw new Error(`recovery asset download failed with HTTP ${response.status}`)
  const bytes = Buffer.from(await response.arrayBuffer()), digest = createHash("sha256").update(bytes).digest("hex")
  if (asset.digest !== `sha256:${digest}` || asset.size !== bytes.length || asset.state !== "uploaded" || !actionsBot(asset.uploader)) throw new Error("recovery asset bytes or uploader are not trusted")
  const path = join(directory, asset.name)
  writeFileSync(path, bytes, { flag: "wx" })
  return { path, digest, size: bytes.length }
}
function verifyAttestation(repo, path, executionSHA) {
  execFileSync("gh", ["attestation", "verify", path, "--repo", repo, "--signer-workflow", `${repo}/.github/workflows/release-recover.yml`, "--signer-digest", executionSHA, "--source-ref", "refs/heads/main", "--source-digest", executionSHA, "--predicate-type", "https://slsa.dev/provenance/v1", "--deny-self-hosted-runners"], { encoding: "utf8", env: { ...process.env, GH_TOKEN: process.env.GH_TOKEN || process.env.GITHUB_TOKEN }, stdio: ["ignore", "pipe", "pipe"] })
}
function snapshot(assets) { return new Map(assets.map((asset) => [asset.name, { digest: asset.digest, size: asset.size, uploader: asset.uploader?.id, state: asset.state }])) }
function assertSnapshot(assets, expected) {
  if (assets.length !== expected.size) throw new Error("recovery asset set changed during final attestation verification")
  for (const asset of assets) {
    const prior = expected.get(asset.name)
    if (!prior || asset.digest !== prior.digest || asset.size !== prior.size || asset.uploader?.id !== prior.uploader || asset.state !== prior.state) throw new Error("recovery asset state changed during final attestation verification")
  }
}
async function assertPublishedSnapshot(repo, releaseId, tag, version, source, expected, expectedTag, expectedAssets) {
  const byId = assertExactPublished(await api(`repos/${repo}/releases/${releaseId}`), version, source, expected)
  const byTag = assertExactPublished(await api(`repos/${repo}/releases/tags/${tag}`), version, source, expected)
  if (byId.id !== releaseId || byTag.id !== releaseId) throw new Error("recovery release identity changed during final verification")
  if (!sameTagState(await readTagState(repo, tag), expectedTag)) throw new Error("release tag changed during final verification")
  assertSnapshot(await pages(`repos/${repo}/releases/${releaseId}/assets`), expectedAssets)
}
async function withdraw(repo, releaseId, tag, tagState) {
  for (let attempt = 1; ; attempt += 1) {
    try { await api(`repos/${repo}/releases/${releaseId}`, { method: "PATCH", body: { draft: true, prerelease: false, make_latest: "false" } }) } catch (error) { console.warn(`post-publication withdrawal PATCH ${attempt} was ambiguous: ${error.message}`) }
    const all = await pages(`repos/${repo}/releases`)
    const publicForTag = all.filter((release) => release?.tag_name === tag && release.draft === false)
    for (const release of publicForTag) {
      try { await api(`repos/${repo}/releases/${release.id}`, { method: "PATCH", body: { draft: true, prerelease: false, make_latest: "false" } }) } catch (error) { console.warn(`replacement withdrawal PATCH ${attempt} was ambiguous: ${error.message}`) }
    }
    const after = await pages(`repos/${repo}/releases`)
    const current = await api(`repos/${repo}/releases/${releaseId}`, { missing: true })
    const byTag = await api(`repos/${repo}/releases/tags/${tag}`, { missing: true })
    const currentTag = await readTagState(repo, tag)
    const publicRemains = after.some((release) => release?.tag_name === tag && release.draft === false) || Boolean(byTag && byTag.draft === false)
    if (current?.draft === true && !publicRemains) {
      if (!sameTagState(currentTag, tagState)) throw new Error("CRITICAL: publication was withdrawn but immutable recovery tag state changed")
      return
    }
    await delay(Math.min(5000, attempt * 500))
  }
}
async function main() {
  const repo = repository(), version = process.env.RECOVERY_VERSION, source = process.env.RECOVERY_SOURCE, executionSHA = process.env.RECOVERY_EXECUTION_SHA
  if (!version || !sha40(source) || !sha40(executionSHA)) throw new Error("post-publication recovery inputs are invalid")
  const tag = versionTag(version), expectedTag = await expectedTagState(source)
  let releaseId = null
  try {
    const release = await api(`repos/${repo}/releases/tags/${tag}`)
    releaseId = release.id
    await assertExecutionMain(repo, executionSHA)
    const expected = await expectedMetadata(repo, source, version)
    assertExactPublished(release, version, source, expected)
    const canonical = await assertCurrentVersionPublished(repo, { version })
    if (canonical.id !== release.id) throw new Error("final recovery verification resolved a different release")
    const actualTag = await readTagState(repo, tag)
    if (!sameTagState(actualTag, expectedTag)) throw new Error("immutable recovery tag object differs from the metadata snapshot")
    const assets = await pages(`repos/${repo}/releases/${release.id}/assets`), expectedNames = expectedReleaseAssetNames(version)
    const names = assets.map((asset) => asset.name).sort()
    if (names.join("\n") !== expectedNames.join("\n") || new Set(names).size !== names.length) throw new Error("final recovery asset set is incomplete or unexpected")
    const initial = snapshot(assets), directory = mkdtempSync(join(tmpdir(), "darkphish-recovery-final-"))
    try {
      for (const asset of assets) {
        const local = await downloadAsset(repo, asset, directory)
        verifyAttestation(repo, local.path, executionSHA)
      }
    } finally { rmSync(directory, { recursive: true, force: true }) }

    await assertPublishedSnapshot(repo, release.id, tag, version, source, expected, expectedTag, initial)
    await assertExecutionMain(repo, executionSHA)
    await assertPublishedSnapshot(repo, release.id, tag, version, source, expected, expectedTag, initial)
    console.log(`Cryptographically verified published recovery ${tag}, including the attested publication receipt.`)
  } catch (error) {
    if (releaseId) {
      await withdraw(repo, releaseId, tag, expectedTag)
      throw new Error(`published recovery was withdrawn after final attestation verification failed: ${error.message}`)
    }
    throw error
  }
}
main().catch((error) => { console.error(error.message); process.exitCode = 1 })
