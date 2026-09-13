import { appendFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs"
import { createHash } from "node:crypto"
import { execFileSync } from "node:child_process"
import { tmpdir } from "node:os"
import { join } from "node:path"
import {
  api, assertCurrentVersionPublished, expectedReleaseAssetNames, generatedPath, greenCommit,
  pages, peelTagToCommit, publicationReceiptName, repository, requiredChecks, versionTag,
} from "../../scripts/release-lib.mjs"
import { verifyCodeQLBaseline } from "../../scripts/codeql-baseline.mjs"
import { verifyReleaseMaintainerReview } from "../../scripts/release-maintainer-review.mjs"

const sha40 = (value) => typeof value === "string" && /^[a-f0-9]{40}$/.test(value)
const actionsBot = (actor) => actor?.login === "github-actions[bot]" && actor?.type === "Bot" && actor?.id === 41898282
const output = (name, value) => { if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, `${name}=${value}\n`) }
const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const timestamp = (value, label) => {
  const parsed = Date.parse(value || "")
  if (!Number.isFinite(parsed)) throw new Error(`${label} timestamp is invalid`)
  return parsed
}
const successfulStep = (job, name) => job?.steps?.some((step) => step.name === name && step.status === "completed" && step.conclusion === "success")

function notesForVersion(changelog, version) {
  const start = changelog.indexOf(`## ${version} - `)
  if (start < 0) throw new Error("release source changelog is missing the release section")
  const end = changelog.indexOf("\n## ", start + 1)
  return changelog.slice(start, end < 0 ? undefined : end).trim()
}
function releaseName(version) { return `Darkphish ${version.split(".").slice(0, 2).join(".")}` }
function releaseBody(notes, source) {
  return `${notes}\n\nSource commit: ${source}\n\n<!-- darkphish-release-source:${source} -->\n\nNative binaries, SHA-256 checksums and SPDX SBOM are attached.`
}
async function sourceText(repo, path, source) {
  const file = await api(`repos/${repo}/contents/${path}?ref=${source}`)
  if (file?.type !== "file" || file.encoding !== "base64" || typeof file.content !== "string") throw new Error(`release source ${path} is unavailable`)
  return Buffer.from(file.content.replace(/\n/g, ""), "base64").toString("utf8")
}
async function expectedMetadata(repo, source, version) {
  const notes = notesForVersion(await sourceText(repo, "CHANGELOG.md", source), version)
  return { name: releaseName(version), body: releaseBody(notes, source) }
}
function assertExactPublished(release, version, source, expected) {
  const tag = versionTag(version)
  if (!release || release.tag_name !== tag || release.target_commitish !== source || release.name !== expected.name || release.body !== expected.body || release.draft !== false || release.prerelease !== false || !release.published_at || !actionsBot(release.author)) {
    throw new Error("published release metadata does not exactly match the verified source")
  }
  return release
}
async function readTagState(repo, tag) {
  const ref = await api(`repos/${repo}/git/ref/tags/${tag}`, { missing: true })
  if (!ref) return null
  const objectType = ref.object?.type
  const objectSha = ref.object?.sha
  if (!sha40(objectSha) || !["commit", "tag"].includes(objectType)) throw new Error("release tag ref object is malformed")
  const commit = await peelTagToCommit(ref, (sha) => api(`repos/${repo}/git/tags/${sha}`))
  return { objectType, objectSha, commit }
}
function sameTagState(actual, expected) {
  return Boolean(actual && expected && actual.objectType === expected.objectType && actual.objectSha === expected.objectSha && actual.commit === expected.commit)
}
async function assertTagState(repo, tag, expected, message) {
  const current = await readTagState(repo, tag)
  if (!sameTagState(current, expected)) throw new Error(message)
  return current
}
async function executionMain(repo, expectedSHA) {
  if (!sha40(expectedSHA)) throw new Error("recovery execution SHA is invalid")
  const metadata = await api(`repos/${repo}`)
  const branch = await api(`repos/${repo}/branches/main`)
  if (metadata?.default_branch !== "main" || metadata?.private !== false || metadata?.fork !== false || branch?.protected !== true || branch?.commit?.sha !== expectedSHA) {
    throw new Error("recovery workflow must execute from the immutable current protected-main SHA")
  }
  if (!await greenCommit(repo, expectedSHA)) throw new Error("recovery execution SHA required checks are not green")
  await verifyCodeQLBaseline(repo, expectedSHA)
  if (!await greenCommit(repo, expectedSHA)) throw new Error("recovery execution SHA required checks changed during CodeQL verification")
  const closing = await api(`repos/${repo}/branches/main`)
  if (!closing?.protected || closing.commit?.sha !== expectedSHA) throw new Error("protected main moved during recovery execution verification")
  return expectedSHA
}
async function verifySourceAncestry(repo, source, main) {
  if (source === main) return
  const comparison = await api(`repos/${repo}/compare/${source}...${main}`)
  if (comparison?.base_commit?.sha !== source || !["ahead", "identical"].includes(comparison?.status) || comparison.behind_by !== 0) throw new Error("release source is not an ancestor of protected main")
}
async function verifiedReleasePR(repo, source, version, tag) {
  if (!await greenCommit(repo, source)) throw new Error("release source required checks are not green")
  const prs = await pages(`repos/${repo}/commits/${source}/pulls`)
  const matches = prs.filter((pr) => pr.merged_at && pr.merge_commit_sha === source && pr.base?.ref === "main" && pr.head?.ref === `release/${tag}` && pr.title === `release: Darkphish ${version}`)
  if (matches.length !== 1) throw new Error("release source does not map to exactly one trusted release PR")
  const pr = await api(`repos/${repo}/pulls/${matches[0].number}`)
  await verifyReleaseMaintainerReview(repo, pr, { get: api })
  const files = await pages(`repos/${repo}/pulls/${pr.number}/files`)
  if (!files.length || files.some((file) => !generatedPath(file.filename))) throw new Error("release PR includes application changes")
}
async function exactMainChecks(repo, source, before = Infinity) {
  const runs = await pages(`repos/${repo}/actions/runs?head_sha=${source}`, "workflow_runs")
  const okay = (run, name, path) => run.name === name && run.path === path && run.event === "push" && run.head_branch === "main" && run.head_sha === source && run.status === "completed" && run.conclusion === "success" && timestamp(run.updated_at, `${name} run`) <= before
  if (!runs.some((run) => okay(run, "CI", ".github/workflows/ci.yml")) || !runs.some((run) => okay(run, "CodeQL", ".github/workflows/codeql.yml"))) throw new Error("publication source lacks canonical exact-SHA CI or CodeQL")
}
async function exactRequiredChecksBefore(repo, source, before) {
  const checks = await pages(`repos/${repo}/commits/${source}/check-runs?filter=all`, "check_runs")
  for (const name of requiredChecks) {
    if (!checks.some((check) => check.name === name && check.app?.slug === "github-actions" && check.status === "completed" && check.conclusion === "success" && timestamp(check.completed_at, `${name} check`) <= before)) {
      throw new Error(`publication source lacks required successful check ${name}`)
    }
  }
}
async function downloadAsset(repo, asset, directory) {
  if (!Number.isSafeInteger(asset?.id) || asset.id < 1 || typeof asset?.name !== "string") throw new Error("release asset metadata is malformed")
  const url = new URL(asset.url || "")
  if (url.protocol !== "https:" || url.hostname !== "api.github.com" || url.pathname !== `/repos/${repo}/releases/assets/${asset.id}`) throw new Error("release asset API URL is unexpected")
  const response = await fetch(url, { method: "GET", headers: { Accept: "application/octet-stream", Authorization: `Bearer ${process.env.GH_TOKEN || process.env.GITHUB_TOKEN}`, "X-GitHub-Api-Version": "2022-11-28" }, redirect: "follow", signal: AbortSignal.timeout(30000) })
  if (!response.ok) throw new Error(`release asset download failed with HTTP ${response.status}`)
  const bytes = Buffer.from(await response.arrayBuffer())
  const digest = createHash("sha256").update(bytes).digest("hex")
  if (asset.digest !== `sha256:${digest}` || asset.size !== bytes.length || !actionsBot(asset.uploader) || asset.state !== "uploaded") throw new Error("release asset bytes or uploader do not match trusted metadata")
  const path = join(directory, asset.name)
  writeFileSync(path, bytes, { flag: "wx" })
  return { path, digest, size: bytes.length }
}
function verifyAttestation(repo, path, workflow, digest, run) {
  if (!sha40(digest)) throw new Error("attestation signer digest is invalid")
  if (!Number.isSafeInteger(run?.id) || run.id < 1 || !Number.isSafeInteger(run?.run_attempt) || run.run_attempt < 1) throw new Error("attestation workflow run identity is invalid")
  const stdout = execFileSync("gh", ["attestation", "verify", path, "--repo", repo, "--signer-workflow", `${repo}/${workflow}`, "--signer-digest", digest, "--source-ref", "refs/heads/main", "--source-digest", digest, "--predicate-type", "https://slsa.dev/provenance/v1", "--deny-self-hosted-runners", "--format", "json"], {
    encoding: "utf8", env: { ...process.env, GH_TOKEN: process.env.GH_TOKEN || process.env.GITHUB_TOKEN }, stdio: ["ignore", "pipe", "pipe"],
  })
  let verified
  try { verified = JSON.parse(stdout) } catch { throw new Error("verified attestation JSON is malformed") }
  const expectedInvocation = `https://github.com/${repo}/actions/runs/${run.id}/attempts/${run.run_attempt}`
  const matching = Array.isArray(verified) && verified.some((entry) => entry?.verificationResult?.statement?.predicate?.runDetails?.metadata?.invocationId === expectedInvocation)
  if (!matching) throw new Error("artifact attestation is not bound to the selected workflow run and attempt")
}
function assetSnapshot(assets) {
  return new Map(assets.map((asset) => [asset.name, { digest: asset.digest, size: asset.size, uploader: asset.uploader?.id, state: asset.state }]))
}
function assertSameAssets(assets, snapshot) {
  if (assets.length !== snapshot.size) throw new Error("release asset set changed during provenance verification")
  for (const asset of assets) {
    const prior = snapshot.get(asset.name)
    if (!prior || asset.digest !== prior.digest || asset.size !== prior.size || asset.uploader?.id !== prior.uploader || asset.state !== prior.state) throw new Error("release asset state changed during provenance verification")
  }
}
async function assertPublishedSnapshot(repo, release, version, tagState, common) {
  const byId = assertExactPublished(await api(`repos/${repo}/releases/${release.id}`), version, common.source, common.expected)
  const byTag = assertExactPublished(await api(`repos/${repo}/releases/tags/${versionTag(version)}`), version, common.source, common.expected)
  if (byId.id !== release.id || byTag.id !== release.id) throw new Error("published release identity changed during final provenance verification")
  await assertTagState(repo, versionTag(version), tagState, "release tag object changed during final provenance verification")
  assertSameAssets(await pages(`repos/${repo}/releases/${release.id}/assets`), common.snapshot)
}
async function commonPublishedState(repo, release, version, main, tagState) {
  const source = release.target_commitish
  if (!sha40(source) || !tagState || tagState.commit !== source) throw new Error("published release tag/source identity is invalid")
  await verifySourceAncestry(repo, source, main)
  await verifiedReleasePR(repo, source, version, versionTag(version))
  const expected = await expectedMetadata(repo, source, version)
  assertExactPublished(release, version, source, expected)
  const canonical = await assertCurrentVersionPublished(repo, { version })
  assertExactPublished(canonical, version, source, expected)
  if (canonical.id !== release.id) throw new Error("published release resolves a different canonical release identity")
  await assertTagState(repo, versionTag(version), tagState, "release tag object changed before provenance verification")
  const assets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  const expectedNames = expectedReleaseAssetNames(version)
  const names = assets.map((asset) => asset.name).sort()
  if (names.join("\n") !== expectedNames.join("\n") || new Set(names).size !== names.length) throw new Error("published release does not contain the exact expected asset set")
  return { source, expected, assets, snapshot: assetSnapshot(assets) }
}
async function verifyRecoveryPublication(repo, release, version, main, tagState, common) {
  const directory = mkdtempSync(join(tmpdir(), "darkphish-recovery-publication-"))
  try {
    const local = new Map()
    for (const asset of common.assets) local.set(asset.name, await downloadAsset(repo, asset, directory))
    const publishedAt = timestamp(release.published_at, "release publication")
    const runs = await pages(`repos/${repo}/actions/workflows/release-recover.yml/runs?branch=main&status=success`, "workflow_runs")
    for (const run of runs.sort((a, b) => b.id - a.id)) {
      try {
        if (run.name !== "Recover pending release" || run.path !== ".github/workflows/release-recover.yml" || run.head_branch !== "main" || !sha40(run.head_sha) || !["workflow_run", "schedule"].includes(run.event) || run.status !== "completed" || run.conclusion !== "success") continue
        const created = timestamp(run.created_at, "recovery run creation")
        await exactMainChecks(repo, run.head_sha, created)
        await exactRequiredChecksBefore(repo, run.head_sha, created)
        const jobs = await pages(`repos/${repo}/actions/runs/${run.id}/jobs`, "jobs")
        const metadata = jobs.find((job) => job.name === "metadata"), verify = jobs.find((job) => job.name === "verify"), smoke = jobs.find((job) => job.name === "audit-smoke"), publish = jobs.find((job) => job.name === "publish"), binaries = jobs.filter((job) => job.name?.startsWith("binaries ("))
        if (metadata?.conclusion !== "success" || verify?.conclusion !== "success" || smoke?.conclusion !== "success" || publish?.conclusion !== "success" || binaries.length !== 5 || binaries.some((job) => job.conclusion !== "success")) continue
        if (!successfulStep(publish, "Attest rebuilt recovery artifacts") || !successfulStep(publish, "Publish rebuilt verified recovery assets")) continue
        const publishStep = publish.steps.find((step) => step.name === "Publish rebuilt verified recovery assets")
        const start = timestamp(publishStep.started_at, "recovery publish start"), end = timestamp(publishStep.completed_at, "recovery publish end")
        if (publishedAt < start || publishedAt > end) continue
        for (const asset of common.assets) verifyAttestation(repo, local.get(asset.name).path, ".github/workflows/release-recover.yml", run.head_sha, run)
        await assertPublishedSnapshot(repo, release, version, tagState, common)
        const finalMain = await executionMain(repo, process.env.RECOVERY_EXECUTION_SHA)
        await verifySourceAncestry(repo, common.source, finalMain)
        await verifySourceAncestry(repo, run.head_sha, finalMain)
        await assertPublishedSnapshot(repo, release, version, tagState, common)
        return run
      } catch (error) { console.warn(`Ignoring non-qualifying recovery publication run ${run.id}: ${error.message}`) }
    }
    throw new Error("public release has no qualifying fully attested recovery publication run")
  } finally { rmSync(directory, { recursive: true, force: true }) }
}
async function verifyNativePublication(repo, release, version, main, tagState, common) {
  const directory = mkdtempSync(join(tmpdir(), "darkphish-native-publication-"))
  try {
    const local = new Map()
    for (const asset of common.assets) local.set(asset.name, await downloadAsset(repo, asset, directory))
    const publishedAt = timestamp(release.published_at, "native release publication")
    const runs = await pages(`repos/${repo}/actions/runs?head_sha=${common.source}`, "workflow_runs")
    for (const run of runs.sort((a, b) => b.id - a.id)) {
      try {
        if (run.name !== "Native release" || run.path !== ".github/workflows/release.yml" || run.head_branch !== "main" || run.head_sha !== common.source || !["workflow_run", "schedule"].includes(run.event) || run.status !== "completed" || run.conclusion !== "success") continue
        const created = timestamp(run.created_at, "native release run creation")
        await exactMainChecks(repo, common.source, created)
        await exactRequiredChecksBefore(repo, common.source, created)
        const jobs = await pages(`repos/${repo}/actions/runs/${run.id}/jobs`, "jobs")
        const metadata = jobs.find((job) => job.name === "metadata"), verify = jobs.find((job) => job.name === "verify"), smoke = jobs.find((job) => job.name === "audit-smoke"), publish = jobs.find((job) => job.name === "publish"), binaries = jobs.filter((job) => job.name?.startsWith("binaries ("))
        if (metadata?.conclusion !== "success" || verify?.conclusion !== "success" || smoke?.conclusion !== "success" || publish?.conclusion !== "success" || binaries.length !== 5 || binaries.some((job) => job.conclusion !== "success")) continue
        if (!successfulStep(metadata, "Run node scripts/release-publish.mjs metadata") || !successfulStep(publish, "Generate checksums") || !successfulStep(publish, "Publish verified assets without overwriting an existing release")) continue
        const publishStep = publish.steps.find((step) => step.name === "Publish verified assets without overwriting an existing release")
        const start = timestamp(publishStep.started_at, "native publish start"), end = timestamp(publishStep.completed_at, "native publish end")
        if (publishedAt < start || publishedAt > end) continue
        const attestStep = publish.steps.find((step) => step.name === "Attest canonical native release artifacts" || step.name === "Run actions/attest@v4")
        if (attestStep?.status !== "completed" || attestStep.conclusion !== "success") throw new Error("canonical Native release did not attest its complete published asset set")
        for (const asset of common.assets) verifyAttestation(repo, local.get(asset.name).path, ".github/workflows/release.yml", common.source, run)
        await assertPublishedSnapshot(repo, release, version, tagState, common)
        const finalMain = await executionMain(repo, process.env.RECOVERY_EXECUTION_SHA)
        await verifySourceAncestry(repo, common.source, finalMain)
        await assertPublishedSnapshot(repo, release, version, tagState, common)
        return run
      } catch (error) { console.warn(`Ignoring non-qualifying native publication run ${run.id}: ${error.message}`) }
    }
    throw new Error("public release has no qualifying canonical fully attested Native release publication run")
  } finally { rmSync(directory, { recursive: true, force: true }) }
}
async function withdrawUnverified(repo, tag, expectedTagState, releases) {
  const ids = new Set(releases.filter((release) => Number.isSafeInteger(release?.id)).map((release) => release.id))
  for (let attempt = 1; ; attempt += 1) {
    const listed = (await pages(`repos/${repo}/releases`)).filter((release) => release?.tag_name === tag && release.draft === false)
    for (const release of listed) ids.add(release.id)

    for (const id of ids) {
      try { await api(`repos/${repo}/releases/${id}`, { method: "PATCH", body: { draft: true, prerelease: false, make_latest: "false" } }) }
      catch (error) { console.warn(`withdrawal PATCH attempt ${attempt} for ${id} was ambiguous: ${error.message}`) }
    }

    const after = await pages(`repos/${repo}/releases`)
    const publicAfter = after.filter((release) => release?.tag_name === tag && release.draft === false)
    for (const release of publicAfter) ids.add(release.id)
    const byTag = await api(`repos/${repo}/releases/tags/${tag}`, { missing: true })
    if (Number.isSafeInteger(byTag?.id)) ids.add(byTag.id)
    const finalDirect = await Promise.all([...ids].map((id) => api(`repos/${repo}/releases/${id}`, { missing: true })))
    const directNotWithdrawn = finalDirect.some((release) => release && (release.draft !== true || release.prerelease !== false))
    const publicByTag = Boolean(byTag && byTag.draft === false)

    if (!directNotWithdrawn && publicAfter.length === 0 && !publicByTag) {
      if (expectedTagState !== undefined) {
        const currentTag = await readTagState(repo, tag)
        if (expectedTagState === null) {
          if (currentTag !== null) throw new Error("release tag appeared while confirming withdrawal of an explicitly tagless publication")
        } else if (!sameTagState(currentTag, expectedTagState)) {
          throw new Error("immutable release tag object changed while withdrawing unverified publication")
        }
      }
      return
    }
    await delay(Math.min(5000, 500 * attempt))
  }
}
function parsePrecheck(value) {
  if (value === "true") return { absent: true, tagState: null }
  const match = /^false:(commit|tag):([a-f0-9]{40})$/.exec(value || "")
  if (!match) throw new Error("missing-tag precheck result is unavailable or invalid")
  return { absent: false, tagState: { objectType: match[1], objectSha: match[2] } }
}
async function preflight() {
  const repo = repository(), version = readFileSync("VERSION", "utf8").trim(), tag = versionTag(version)
  const executionSHA = process.env.RECOVERY_EXECUTION_SHA
  const precheck = parsePrecheck(process.env.RECOVERY_PRECHECK_TAG_ABSENT)
  const main = await executionMain(repo, executionSHA)
  const releases = await pages(`repos/${repo}/releases`)
  const publicReleases = releases.filter((release) => release?.tag_name === tag && release.draft === false)
  const tagState = await readTagState(repo, tag)

  if (precheck.absent && tagState) {
    if (publicReleases.length) await withdrawUnverified(repo, tag, undefined, publicReleases)
    throw new Error("release tag appeared after the explicit absent-tag precheck; refusing to establish a recovery trust baseline")
  }

  if (!precheck.absent) {
    if (!tagState) {
      if (publicReleases.length) await withdrawUnverified(repo, tag, undefined, publicReleases)
      throw new Error("release tag disappeared after the tagged precheck; refusing to re-baseline recovery trust")
    }
    if (tagState.objectType !== precheck.tagState.objectType || tagState.objectSha !== precheck.tagState.objectSha) {
      if (publicReleases.length) await withdrawUnverified(repo, tag, undefined, publicReleases)
      throw new Error("release tag object changed after the tagged precheck; refusing to re-baseline recovery trust")
    }
  }

  if (precheck.absent && publicReleases.length) {
    await withdrawUnverified(repo, tag, null, publicReleases)
    throw new Error("tagless public release appeared after the absent-tag precheck; publication was withdrawn and recovery failed closed")
  }

  if (!publicReleases.length) {
    const finalTag = await readTagState(repo, tag)
    if (precheck.absent && finalTag) throw new Error("release tag appeared after the absent-tag precheck")
    if (!precheck.absent && (!finalTag || finalTag.objectType !== precheck.tagState.objectType || finalTag.objectSha !== precheck.tagState.objectSha)) throw new Error("release tag object changed after the tagged precheck")
    output("verified", "false")
    return
  }

  if (publicReleases.length === 1) {
    const release = publicReleases[0]
    try {
      const common = await commonPublishedState(repo, release, version, main, tagState)
      try {
        const run = await verifyRecoveryPublication(repo, release, version, main, tagState, common)
        output("verified", "true"); output("kind", "recovery")
        console.log(`Verified existing hardened recovery publication ${tag} from run ${run.id}; leaving it public.`)
        return
      } catch (recoveryError) {
        const run = await verifyNativePublication(repo, release, version, main, tagState, common)
        output("verified", "true"); output("kind", "native")
        console.log(`Verified existing canonical fully attested Native release publication ${tag} from run ${run.id}; leaving it public.`)
        return
      }
    } catch (error) { console.warn(`Existing public ${tag} failed both trusted publication paths and will be withdrawn: ${error.message}`) }
  }

  await withdrawUnverified(repo, tag, tagState, publicReleases)
  output("verified", "false")
  console.warn(`Withdrew unverified public ${tag}; normal fail-closed recovery may proceed.`)
}
function generateReceipt() {
  const version = process.env.RECOVERY_VERSION, source = process.env.RECOVERY_SOURCE
  if (!version || !sha40(source)) throw new Error("recovery receipt inputs are invalid")
  const tag = versionTag(version), name = publicationReceiptName(version)
  if (!name) throw new Error("recovery publication receipt is unsupported")
  const checksums = readFileSync("dist/SHA256SUMS")
  const receipt = Buffer.from(`${JSON.stringify({ schema: "darkphish-release-publication-receipt/v1", tag, source_sha: source, checksums_sha256: createHash("sha256").update(checksums).digest("hex") })}\n`, "utf8")
  const directory = ".cache/recovery-receipt"
  mkdirSync(directory, { recursive: true })
  writeFileSync(join(directory, name), receipt, { flag: "wx" })
  console.log(`Prepared immutable publication receipt ${name} for attestation.`)
}
async function verifyExecution() {
  await executionMain(repository(), process.env.RECOVERY_EXECUTION_SHA)
  console.log("Verified immutable current protected-main execution before release mutation.")
}
const command = process.argv[2] || "preflight"
const runner = command === "preflight" ? preflight : command === "receipt" ? async () => generateReceipt() : command === "execution" ? verifyExecution : null
if (!runner) throw new Error(`unknown publication guard command ${command}`)
runner().catch((error) => { console.error(error.message); process.exitCode = 1 })
