import { execFileSync } from "node:child_process"
import { readFileSync } from "node:fs"
import { ReviewGateError, verifyPullRequestReviews } from "./review-gate.mjs"

export const requiredChecks = JSON.parse(readFileSync(new URL("../.github/required-checks.json", import.meta.url), "utf8"))
export const generatedPath = (path) => path === "VERSION" || path === "CHANGELOG.md" || /^changes\/[a-z0-9][a-z0-9-]*\.md$/.test(path)
export const versionTag = (value) => {
  if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(value)) throw new Error("release VERSION must be stable SemVer")
  return `v${value}`
}
const versionAtLeast = (value, floor) => {
  versionTag(value); versionTag(floor)
  const a = value.split(".").map(Number), b = floor.split(".").map(Number)
  for (let i = 0; i < 3; i += 1) if (a[i] !== b[i]) return a[i] > b[i]
  return true
}
export function nextPatchVersion(value) {
  versionTag(value)
  const [major, minor, patch] = value.split(".").map(Number)
  return `${major}.${minor}.${patch + 1}`
}
export function publicationReceiptName(version) {
  return versionAtLeast(version, "0.7.1") ? `darkphish-v${version}.release.json` : null
}
export function expectedReleaseAssetNames(version) {
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
const githubActionsBot = (actor) => actor?.login === "github-actions[bot]" && actor?.type === "Bot" && actor?.id === 41898282
const githubPublishedAt = (value) => {
  if (typeof value !== "string") return false
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?Z$/)
  if (!match) return false
  const [, ys, mos, ds, hs, mis, ss] = match
  const year = Number(ys), month = Number(mos), day = Number(ds), hour = Number(hs), minute = Number(mis), second = Number(ss)
  if (month < 1 || month > 12 || day < 1 || hour > 23 || minute > 59 || second > 59) return false
  const instant = new Date(Date.UTC(year, month - 1, day, hour, minute, second))
  return instant.getUTCFullYear() === year && instant.getUTCMonth() === month - 1 && instant.getUTCDate() === day && instant.getUTCHours() === hour && instant.getUTCMinutes() === minute && instant.getUTCSeconds() === second
}
export async function peelTagToCommit(ref, fetchTag, maxDepth = 8) {
  if (!Number.isSafeInteger(maxDepth) || maxDepth < 1 || maxDepth > 32) throw new Error("invalid tag peel depth")
  let object = ref?.object
  const seen = new Set()
  for (let depth = 0; depth <= maxDepth; depth++) {
    if (!object || !/^[a-f0-9]{40}$/.test(object.sha || "") || !["commit", "tag", "tree", "blob"].includes(object.type)) throw new Error("release tag has malformed Git object metadata")
    if (object.type === "commit") return object.sha
    if (object.type !== "tag") throw new Error("release tag must resolve to a commit")
    if (seen.has(object.sha)) throw new Error("release tag contains a cycle")
    if (depth === maxDepth) throw new Error("release tag exceeds maximum peel depth")
    seen.add(object.sha)
    const tag = await fetchTag(object.sha)
    object = tag?.object
  }
  throw new Error("release tag does not resolve to a commit")
}
export function assertPublishedVersion(tagSHA, release, version) {
  const tag = versionTag(version), expectedNames = expectedReleaseAssetNames(version)
  const source = release?.target_commitish
  if (release?.tag_name !== tag || release.draft !== false || release.prerelease !== false || !githubPublishedAt(release?.published_at) || !/^[a-f0-9]{40}$/.test(source || "") || !githubActionsBot(release?.author)) throw new Error(`patch release requires trusted published current version ${tag}`)
  if (assertReleaseState(tagSHA, release, source) !== "published") throw new Error(`patch release requires trusted published current version ${tag}`)
  const assets = release.assets
  if (!Array.isArray(assets) || assets.length !== expectedNames.length) throw new Error(`patch release requires complete trusted artifact set for ${tag}`)
  const names = assets.map((asset) => asset?.name)
  if (new Set(names).size !== names.length || names.slice().sort().join("\n") !== expectedNames.join("\n")) throw new Error(`patch release requires exact trusted artifact set for ${tag}`)
  for (const asset of assets) {
    if (asset?.state !== "uploaded" || !githubActionsBot(asset?.uploader) || !/^sha256:[a-f0-9]{64}$/.test(asset?.digest || "") || !Number.isSafeInteger(asset?.size) || asset.size <= 0) throw new Error(`patch release requires trusted uploaded artifacts for ${tag}`)
  }
  return release
}
async function downloadReleaseAsset(asset, suffix, download) {
  if (!asset || typeof asset.browser_download_url !== "string") throw new Error(`published release ${suffix} is unavailable`)
  const url = new URL(asset.browser_download_url)
  if (url.protocol !== "https:" || url.hostname !== "github.com" || !url.pathname.endsWith(`/${asset.name}`)) throw new Error(`published release ${suffix} has an unexpected download URL`)
  const response = await download(url, { method: "GET", redirect: "follow", signal: AbortSignal.timeout(15000) })
  if (!response.ok) throw new Error(`published release ${suffix} download failed with HTTP ${response.status}`)
  return response.text()
}
export async function verifyPublishedAssetManifest(release, { download = fetch } = {}) {
  const assets = release?.assets
  const manifest = Array.isArray(assets) ? assets.find((asset) => asset?.name === "SHA256SUMS") : null
  const text = await downloadReleaseAsset(manifest, "checksum manifest", download)
  if (text.length > 65536) throw new Error("published release checksum manifest is unexpectedly large")
  const expected = new Map()
  for (const line of text.trim().split(/\r?\n/)) {
    const match = line.match(/^([a-f0-9]{64})  (darkphish-[A-Za-z0-9._-]+)$/)
    if (!match || expected.has(match[2])) throw new Error("published release checksum manifest is malformed")
    expected.set(match[2], match[1])
  }
  const payloads = assets.filter((asset) => asset.name !== "SHA256SUMS" && !asset.name.endsWith(".release.json"))
  if (expected.size !== payloads.length) throw new Error("published release checksum manifest is incomplete")
  for (const asset of payloads) if (expected.get(asset.name) !== asset.digest?.replace(/^sha256:/, "")) throw new Error("published release asset name is not cryptographically bound to its digest")
  return true
}
export async function verifyPublicationReceipt(release, version, { download = fetch } = {}) {
  const name = publicationReceiptName(version)
  if (!name) return true
  const receiptAsset = release?.assets?.find((asset) => asset?.name === name)
  const text = await downloadReleaseAsset(receiptAsset, "publication receipt", download)
  if (text.length > 4096) throw new Error("published release publication receipt is unexpectedly large")
  let receipt
  try { receipt = JSON.parse(text) } catch { throw new Error("published release publication receipt is malformed") }
  const manifest = release.assets.find((asset) => asset?.name === "SHA256SUMS")
  const expected = {
    schema: "darkphish-release-publication-receipt/v1",
    tag: versionTag(version),
    source_sha: release.target_commitish,
    checksums_sha256: manifest?.digest?.replace(/^sha256:/, ""),
  }
  if (Object.keys(receipt).sort().join("\n") !== Object.keys(expected).sort().join("\n") || Object.entries(expected).some(([key, value]) => receipt[key] !== value) || !/^[a-f0-9]{64}$/.test(expected.checksums_sha256 || "")) throw new Error("published release publication receipt does not prove the final gated artifact set")
  return true
}
export async function assertCurrentVersionPublished(repo, { request = api, version = readFileSync("VERSION", "utf8").trim(), verifyManifest = true } = {}) {
  const tag = versionTag(version)
  const ref = await request(`repos/${repo}/git/ref/tags/${tag}`, { missing: true })
  const tagSHA = ref ? await peelTagToCommit(ref, (sha) => request(`repos/${repo}/git/tags/${sha}`)) : null
  const release = await request(`repos/${repo}/releases/tags/${tag}`, { missing: true })
  assertPublishedVersion(tagSHA, release, version)
  if (verifyManifest) {
    await verifyPublishedAssetManifest(release)
    await verifyPublicationReceipt(release, version)
  }
  return release
}
export function checksPassed(checks, names = requiredChecks) {
  return names.every((name) => {
    const latest = checks.filter((check) => check.name === name && check.app?.slug === "github-actions").sort((a, b) => b.id - a.id)[0]
    return latest?.status === "completed" && latest.conclusion === "success"
  })
}
export function assertGeneratedCommits(commits, files, version) {
  if (!commits.length || commits.some((commit) => commit.author !== "github-actions[bot]" || commit.subject !== `release: Darkphish ${version}`)) throw new Error("release branch contains unknown commits; refusing to overwrite developer work")
  if (!files.length || files.some((file) => !generatedPath(file))) throw new Error("release branch contains non-generated changes")
}
export function assertReleaseState(tagSHA, release, sha) {
  if (!tagSHA && !release) return "new"
  if (!tagSHA && release?.draft && release.target_commitish === sha && release.body?.includes(`<!-- darkphish-release-source:${sha} -->`)) return "resume"
  if (tagSHA !== sha) throw new Error("release tag already exists at an unexpected commit; tags are immutable")
  if (!release || !release.body?.includes(`<!-- darkphish-release-source:${sha} -->`)) throw new Error("existing release state is not a known automation artifact")
  return release.draft ? "resume" : "published"
}
export function assetDisposition(existing, digest, size) {
  if (!existing) return "upload"
  if (existing.digest !== `sha256:${digest}` || existing.size !== size) throw new Error("existing release asset differs; refusing to replace a released binary")
  return "reuse"
}
export function protectedMergeRequest(repo, pr) {
  if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repo) || repo.split("/").some(part => part === "." || part === "..") || !Number.isSafeInteger(pr.number) || pr.number < 1 || !/^[a-f0-9]{40}$/.test(pr.head?.sha || "")) throw new Error("invalid protected merge target")
  if (pr.draft !== false || pr.state !== "open" || pr.base?.ref !== "main" || pr.head.repo?.full_name !== repo) throw new Error("only internal ready main pull requests are eligible")
  return { path: `repos/${repo}/pulls/${pr.number}/merge`, method: "PUT", body: { sha: pr.head.sha, merge_method: "squash" } }
}
export function verifyChecksums(manifest, hashes) {
  const seen = new Set(), lines = manifest.trim().split(/\r?\n/)
  for (const line of lines) {
    const match = line.match(/^([a-f0-9]{64})  (darkphish-[A-Za-z0-9._-]+)$/)
    if (!match || seen.has(match[2]) || hashes.get(match[2]) !== match[1]) throw new Error("release checksum manifest does not match the native artifacts")
    seen.add(match[2])
  }
  if (seen.size !== hashes.size - 1 || [...hashes.keys()].some((name) => name !== "SHA256SUMS" && !seen.has(name))) throw new Error("release checksum manifest is incomplete")
}
export function git(...args) { return execFileSync("git", args, { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim() }
export function repository() {
  const repo = process.env.GITHUB_REPOSITORY || process.env.GH_REPO
  if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repo || "")) throw new Error("GITHUB_REPOSITORY is required")
  return repo
}
export async function api(path, { method = "GET", body, missing = false } = {}) {
  const response = await fetch(`https://api.github.com/${path}`, { method, redirect: "error", signal: AbortSignal.timeout(15000), headers: { Accept: "application/vnd.github+json", Authorization: `Bearer ${process.env.GH_TOKEN || process.env.GITHUB_TOKEN}`, "X-GitHub-Api-Version": "2022-11-28" }, body: body === undefined ? undefined : JSON.stringify(body) })
  if (response.status === 404 && missing) return null
  if (!response.ok) { const error = new Error(`GitHub ${method} ${path.split("?")[0]} returned HTTP ${response.status}`); error.status = response.status; throw error }
  return response.status === 204 ? null : response.json()
}
export async function pages(path, key, request = api) {
  const all = []
  for (let page = 1; page <= 100; page++) {
    const value = await request(`${path}${path.includes("?") ? "&" : "?"}per_page=100&page=${page}`), rows = key ? value[key] : value
    if (!Array.isArray(rows) || rows.length > 100) throw new Error("invalid GitHub API page")
    all.push(...rows)
    if (rows.length < 100) return all
  }
  throw new Error("GitHub pagination limit reached; manual investigation required")
}
export async function protectedMain(repo, sha) {
  const metadata = await api(`repos/${repo}`)
  if (metadata.default_branch !== "main" || metadata.private !== false || metadata.fork !== false) throw new Error("expected standalone public repository with default main")
  const branch = await api(`repos/${repo}/branches/main`)
  if (!branch.protected || branch.commit.sha !== sha) throw new Error("release source must be the current protected main commit")
  return metadata
}
export async function greenCommit(repo, sha) { return checksPassed(await pages(`repos/${repo}/commits/${sha}/check-runs?filter=latest`, "check_runs")) }
export async function mergeReviewedPullRequest(repo, expected, { request = api, verifyReviews = verifyPullRequestReviews, cancelQueued = args => execFileSync("gh", args, { stdio: "inherit" }), log = console.log, releaseMerge = false } = {}) {
  protectedMergeRequest(repo, { ...expected, state: "open", draft: false })
  const path = `repos/${repo}/pulls/${expected.number}`, pr = await request(path)
  if (pr?.number !== expected.number) throw new Error("GitHub returned an inconsistent PR identity")
  const wait = reason => { log(`PR #${expected.number}: ${reason}; no merge queued.`); return false }
  if (pr.state !== "open") return wait("already closed")
  if (pr.head?.repo?.full_name !== repo || pr.base?.ref !== "main") return wait("target changed")
  const metadata = await request(`repos/${repo}`), base = await request(`repos/${repo}/branches/main`)
  if (metadata.full_name !== repo || metadata.default_branch !== "main" || metadata.private !== false || metadata.fork !== false || metadata.allow_auto_merge !== true || base.protected !== true) throw new Error("reviewed merging requires the protected standalone public main repository")
  if (pr.auto_merge) {
    await cancelQueued(["pr", "merge", String(pr.number), "--repo", repo, "--disable-auto"])
    log(`PR #${pr.number}: revoked legacy queued auto-merge before evaluating any release boundary.`)
  }
  const enforceReleaseBoundary = !releaseMerge && request === api
  if (enforceReleaseBoundary) {
    try { await assertCurrentVersionPublished(repo, { request, verifyManifest: false }) } catch { return wait("current VERSION is not fully published; protected main is frozen until release completion") }
  }
  if (pr.head.sha !== expected.head.sha) return wait("head changed")
  if (!/^[a-f0-9]{40}$/.test(base.commit?.sha || "") || pr.base.sha !== base.commit.sha) return wait("base snapshot changed")
  const optedIn = value => value.draft === false && value.labels?.some(label => label.name === "codex-automerge")
  if (!optedIn(pr)) return wait("draft or auto-merge label absent")
  const green = async () => checksPassed(await pages(`repos/${repo}/commits/${pr.head.sha}/check-runs?filter=latest`, "check_runs", request))
  if (!await green()) return wait("required checks pending or unsuccessful")
  if ((await pages(`repos/${repo}/code-scanning/alerts?tool_name=CodeQL&state=open`, undefined, request)).length) return wait("open CodeQL alerts")
  try { await verifyReviews(repo, pr, { get: request, query: body => request("graphql", { method: "POST", body }) }) } catch (error) { if (!(error instanceof ReviewGateError)) throw error; return wait(error.message) }
  const finalPR = await request(path), finalBase = await request(`repos/${repo}/branches/main`)
  if (finalPR.number === pr.number && finalPR.auto_merge && finalPR.head?.repo?.full_name === repo && finalPR.base?.ref === "main") {
    await cancelQueued(["pr", "merge", String(pr.number), "--repo", repo, "--disable-auto"])
    return wait("revoked a concurrent native auto-merge queue")
  }
  if (finalPR.number !== pr.number || finalPR.state !== "open" || finalPR.head?.sha !== pr.head.sha || finalPR.head.repo?.full_name !== repo || finalPR.base?.ref !== "main" || finalPR.base.sha !== pr.base.sha || finalBase.commit?.sha !== base.commit?.sha || finalBase.protected !== true || !optedIn(finalPR) || finalPR.mergeable !== true || finalPR.mergeable_state !== "clean") return wait("PR or protected base is not stably ready")
  if (!await green()) return wait("checks changed during review verification")
  if (enforceReleaseBoundary) {
    try { await assertCurrentVersionPublished(repo, { request, verifyManifest: false }) } catch { return wait("release state changed before merge; protected main remains frozen") }
  }
  const merge = protectedMergeRequest(repo, finalPR), result = await request(merge.path, { method: merge.method, body: merge.body })
  if (result?.merged !== true || !/^[a-f0-9]{40}$/.test(result.sha || "")) throw new Error("GitHub did not confirm the protected merge; inspect current PR state before retrying")
  log(`PR #${pr.number}: protected reviewed squash merge ${result.sha}, expected head ${pr.head.sha}.`)
  return true
}
