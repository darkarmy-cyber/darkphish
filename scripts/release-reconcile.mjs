import { createHash } from "node:crypto"
import { execFileSync } from "node:child_process"
import { mkdtempSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import {
  api, expectedReleaseAssetNames, generatedPath, greenCommit, pages, peelTagToCommit,
  requiredChecks, versionTag,
} from "./release-lib.mjs"
import { matchesTrustedReleaseBody } from "./release-body-match.mjs"
import { releaseBody } from "./release-notes.mjs"
import { verifyCodeQLBaseline } from "./codeql-baseline.mjs"
import { verifyReleaseMaintainerReview } from "./release-maintainer-review.mjs"

const bot = (actor) => actor?.login === "github-actions[bot]" && actor?.type === "Bot" && actor?.id === 41898282
const semver = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/
const sha40 = (value) => typeof value === "string" && /^[a-f0-9]{40}$/.test(value)
const successfulStep = (job, name) => job?.steps?.some((step) => step.name === name && step.status === "completed" && step.conclusion === "success")
const timestamp = (value, label) => {
  const parsed = Date.parse(value || "")
  if (!Number.isFinite(parsed)) throw new Error(`${label} timestamp is invalid`)
  return parsed
}

async function executionMain(repo, expectedSHA) {
  if (!sha40(expectedSHA)) throw new Error("reconciliation execution SHA is invalid")
  const metadata = await api(`repos/${repo}`)
  const branch = await api(`repos/${repo}/branches/main`)
  if (metadata?.default_branch !== "main" || metadata?.private !== false || metadata?.fork !== false || branch?.protected !== true || branch?.commit?.sha !== expectedSHA) {
    throw new Error("reconciliation must execute from the immutable current protected-main SHA")
  }
  if (!await greenCommit(repo, expectedSHA)) throw new Error("reconciliation execution SHA required checks are not green")
  await verifyCodeQLBaseline(repo, expectedSHA)
  if (!await greenCommit(repo, expectedSHA)) throw new Error("reconciliation execution SHA required checks changed during CodeQL verification")
  const closing = await api(`repos/${repo}/branches/main`)
  if (!closing?.protected || closing.commit?.sha !== expectedSHA) throw new Error("protected main moved during reconciliation verification")
  return expectedSHA
}

async function readTagState(repo, tag) {
  const ref = await api(`repos/${repo}/git/ref/tags/${tag}`)
  const objectType = ref?.object?.type, objectSha = ref?.object?.sha
  if (!sha40(objectSha) || !["commit", "tag"].includes(objectType)) throw new Error(`${tag}: release tag metadata is malformed`)
  const commit = await peelTagToCommit(ref, (sha) => api(`repos/${repo}/git/tags/${sha}`))
  return { objectType, objectSha, commit }
}

function sameTagState(actual, expected) {
  return actual?.objectType === expected?.objectType && actual?.objectSha === expected?.objectSha && actual?.commit === expected?.commit
}

async function sourceText(repo, source) {
  const file = await api(`repos/${repo}/contents/CHANGELOG.md?ref=${source}`)
  if (file?.type !== "file" || file.encoding !== "base64" || typeof file.content !== "string") throw new Error("release source changelog is unavailable")
  return Buffer.from(file.content.replace(/\n/g, ""), "base64").toString("utf8")
}

function legacyReleaseBody(changelog, version, source) {
  const start = changelog.indexOf(`## ${version} - `)
  if (start < 0) throw new Error(`changelog is missing release ${version}`)
  const end = changelog.indexOf("\n## ", start + 1)
  const notes = changelog.slice(start, end < 0 ? undefined : end).trim()
  return `${notes}\n\nSource commit: ${source}\n\n<!-- darkphish-release-source:${source} -->\n\nNative binaries, SHA-256 checksums and SPDX SBOM are attached.`
}

async function verifySourceAncestry(repo, source, main) {
  if (source === main) return
  const comparison = await api(`repos/${repo}/compare/${source}...${main}`)
  if (comparison?.base_commit?.sha !== source || !["ahead", "identical"].includes(comparison?.status) || comparison.behind_by !== 0) throw new Error("release source is not an ancestor of protected main")
}

async function verifiedReleasePR(repo, source, version, tag) {
  if (!await greenCommit(repo, source)) throw new Error(`${tag}: release source required checks are not green`)
  const prs = await pages(`repos/${repo}/commits/${source}/pulls`)
  const matches = prs.filter((pr) => pr.merged_at && pr.merge_commit_sha === source && pr.base?.ref === "main" && pr.head?.ref === `release/${tag}` && pr.title === `release: Darkphish ${version}`)
  if (matches.length !== 1) throw new Error(`${tag}: release source does not map to exactly one trusted release PR`)
  const pr = await api(`repos/${repo}/pulls/${matches[0].number}`)
  await verifyReleaseMaintainerReview(repo, pr, { get: api })
  const files = await pages(`repos/${repo}/pulls/${pr.number}/files`)
  if (!files.length || files.some((file) => !generatedPath(file.filename))) throw new Error(`${tag}: release PR includes application changes`)
}

async function exactChecksBefore(repo, source, before) {
  const runs = await pages(`repos/${repo}/actions/runs?head_sha=${source}`, "workflow_runs")
  const runOkay = (run, name, path) => run.name === name && run.path === path && run.event === "push" && run.head_branch === "main" && run.head_sha === source && run.status === "completed" && run.conclusion === "success" && timestamp(run.updated_at, `${name} run`) <= before
  if (!runs.some((run) => runOkay(run, "CI", ".github/workflows/ci.yml")) || !runs.some((run) => runOkay(run, "CodeQL", ".github/workflows/codeql.yml"))) throw new Error("release source lacks exact-SHA CI or CodeQL before publication")
  const checks = await pages(`repos/${repo}/commits/${source}/check-runs?filter=all`, "check_runs")
  for (const name of requiredChecks) {
    if (!checks.some((check) => check.name === name && check.app?.slug === "github-actions" && check.status === "completed" && check.conclusion === "success" && timestamp(check.completed_at, `${name} check`) <= before)) throw new Error(`release source lacks required successful check ${name}`)
  }
}

function assertAssetMetadata(assets, version, tag) {
  const expected = expectedReleaseAssetNames(version)
  if (!Array.isArray(assets) || assets.length !== expected.length) throw new Error(`${tag}: release asset set is incomplete`)
  const names = assets.map((asset) => asset?.name).sort()
  if (new Set(names).size !== names.length || names.join("\n") !== expected.join("\n")) throw new Error(`${tag}: release asset names differ from the trusted set`)
  for (const asset of assets) {
    if (!Number.isSafeInteger(asset?.id) || asset.id < 1 || asset?.state !== "uploaded" || !bot(asset.uploader) || !/^sha256:[a-f0-9]{64}$/.test(asset?.digest || "") || !Number.isSafeInteger(asset?.size) || asset.size <= 0) throw new Error(`${tag}: release asset metadata is untrusted`)
  }
}

async function downloadAssets(repo, assets, tag) {
  const directory = mkdtempSync(join(tmpdir(), "darkphish-release-reconcile-"))
  const local = new Map()
  try {
    for (let index = 0; index < assets.length; index += 1) {
      const asset = assets[index]
      const expectedURL = `https://api.github.com/repos/${repo}/releases/assets/${asset.id}`
      if (asset.url !== expectedURL) throw new Error(`${tag}: release asset API identity is malformed`)
      const response = await fetch(expectedURL, { method: "GET", headers: { Accept: "application/octet-stream", Authorization: `Bearer ${process.env.GH_TOKEN || process.env.GITHUB_TOKEN}`, "X-GitHub-Api-Version": "2022-11-28" }, redirect: "follow", signal: AbortSignal.timeout(30000) })
      if (!response.ok) throw new Error(`${tag}: release asset download failed with HTTP ${response.status}`)
      const bytes = Buffer.from(await response.arrayBuffer())
      const digest = createHash("sha256").update(bytes).digest("hex")
      if (asset.digest !== `sha256:${digest}` || asset.size !== bytes.length) throw new Error(`${tag}: downloaded asset bytes differ from trusted metadata`)
      const path = join(directory, `asset-${index}`)
      writeFileSync(path, bytes, { flag: "wx" })
      local.set(asset.name, { path, digest, size: bytes.length })
    }
    return { directory, local }
  } catch (error) {
    rmSync(directory, { recursive: true, force: true })
    throw error
  }
}

function verifyAttestation(repo, local, workflow, signerSHA, run) {
  if (!sha40(signerSHA) || !Number.isSafeInteger(run?.id) || run.id < 1 || !Number.isSafeInteger(run?.run_attempt) || run.run_attempt < 1) throw new Error("publication attestation identity is malformed")
  const stdout = execFileSync("gh", ["attestation", "verify", local.path, "--repo", repo, "--signer-workflow", `${repo}/${workflow}`, "--signer-digest", signerSHA, "--source-ref", "refs/heads/main", "--source-digest", signerSHA, "--predicate-type", "https://slsa.dev/provenance/v1", "--deny-self-hosted-runners", "--format", "json"], { encoding: "utf8", env: { ...process.env, GH_TOKEN: process.env.GH_TOKEN || process.env.GITHUB_TOKEN }, stdio: ["ignore", "pipe", "pipe"] })
  let verified
  try { verified = JSON.parse(stdout) } catch { throw new Error("verified publication attestation JSON is malformed") }
  const invocation = `https://github.com/${repo}/actions/runs/${run.id}/attempts/${run.run_attempt}`
  if (!Array.isArray(verified) || !verified.some((entry) => entry?.verificationResult?.statement?.predicate?.runDetails?.metadata?.invocationId === invocation)) throw new Error("publication attestation is not bound to the selected workflow run and attempt")
}

async function verifyNativePublication(repo, release, source, assets, local) {
  const publishedAt = timestamp(release.published_at, `${release.tag_name} publication`)
  const runs = await pages(`repos/${repo}/actions/runs?head_sha=${source}`, "workflow_runs")
  for (const run of runs.sort((a, b) => b.id - a.id)) {
    try {
      if (run.name !== "Native release" || run.path !== ".github/workflows/release.yml" || run.head_branch !== "main" || run.head_sha !== source || !["workflow_run", "schedule"].includes(run.event) || run.status !== "completed" || run.conclusion !== "success") continue
      const created = timestamp(run.created_at, "native release creation")
      await exactChecksBefore(repo, source, created)
      const jobs = await pages(`repos/${repo}/actions/runs/${run.id}/jobs`, "jobs")
      const metadata = jobs.find((job) => job.name === "metadata"), verify = jobs.find((job) => job.name === "verify"), smoke = jobs.find((job) => job.name === "audit-smoke"), publish = jobs.find((job) => job.name === "publish"), binaries = jobs.filter((job) => job.name?.startsWith("binaries ("))
      if (metadata?.conclusion !== "success" || verify?.conclusion !== "success" || smoke?.conclusion !== "success" || publish?.conclusion !== "success" || binaries.length !== 5 || binaries.some((job) => job.conclusion !== "success")) continue
      if (!successfulStep(metadata, "Run node scripts/release-publish.mjs metadata") || !successfulStep(publish, "Generate checksums") || !successfulStep(publish, "Publish verified assets without overwriting an existing release")) continue
      const publishStep = publish.steps.find((step) => step.name === "Publish verified assets without overwriting an existing release")
      if (publishedAt < timestamp(publishStep?.started_at, "native publish start") || publishedAt > timestamp(publishStep?.completed_at, "native publish end")) continue
      const attest = publish.steps.find((step) => step.name === "Attest canonical native release artifacts" || step.name === "Run actions/attest@v4")
      if (attest?.status !== "completed" || attest.conclusion !== "success") continue
      for (const asset of assets) verifyAttestation(repo, local.get(asset.name), ".github/workflows/release.yml", source, run)
      return run
    } catch (error) { console.warn(`${release.tag_name}: ignoring non-qualifying Native release run ${run.id}: ${error.message}`) }
  }
  throw new Error(`${release.tag_name}: no qualifying fully attested Native release publication run`)
}

async function verifyRecoveryPublication(repo, release, main, assets, local) {
  const publishedAt = timestamp(release.published_at, `${release.tag_name} publication`)
  const runs = await pages(`repos/${repo}/actions/workflows/release-recover.yml/runs?branch=main&status=success`, "workflow_runs")
  for (const run of runs.sort((a, b) => b.id - a.id)) {
    try {
      if (run.name !== "Recover pending release" || run.path !== ".github/workflows/release-recover.yml" || run.head_branch !== "main" || !sha40(run.head_sha) || !["workflow_run", "schedule"].includes(run.event) || run.status !== "completed" || run.conclusion !== "success") continue
      await verifySourceAncestry(repo, run.head_sha, main)
      await exactChecksBefore(repo, run.head_sha, timestamp(run.created_at, "recovery run creation"))
      const jobs = await pages(`repos/${repo}/actions/runs/${run.id}/jobs`, "jobs")
      const metadata = jobs.find((job) => job.name === "metadata"), verify = jobs.find((job) => job.name === "verify"), smoke = jobs.find((job) => job.name === "audit-smoke"), publish = jobs.find((job) => job.name === "publish"), binaries = jobs.filter((job) => job.name?.startsWith("binaries ("))
      if (metadata?.conclusion !== "success" || verify?.conclusion !== "success" || smoke?.conclusion !== "success" || publish?.conclusion !== "success" || binaries.length !== 5 || binaries.some((job) => job.conclusion !== "success")) continue
      if (!successfulStep(publish, "Attest rebuilt recovery artifacts") || !successfulStep(publish, "Publish rebuilt verified recovery assets")) continue
      const publishStep = publish.steps.find((step) => step.name === "Publish rebuilt verified recovery assets")
      if (publishedAt < timestamp(publishStep?.started_at, "recovery publish start") || publishedAt > timestamp(publishStep?.completed_at, "recovery publish end")) continue
      for (const asset of assets) verifyAttestation(repo, local.get(asset.name), ".github/workflows/release-recover.yml", run.head_sha, run)
      return run
    } catch (error) { console.warn(`${release.tag_name}: ignoring non-qualifying recovery run ${run.id}: ${error.message}`) }
  }
  throw new Error(`${release.tag_name}: no qualifying fully attested recovery publication run`)
}

async function verifyPublicationProvenance(repo, release, version, source, main, assets) {
  await verifySourceAncestry(repo, source, main)
  await verifiedReleasePR(repo, source, version, release.tag_name)
  const { directory, local } = await downloadAssets(repo, assets, release.tag_name)
  try {
    try { return await verifyNativePublication(repo, release, source, assets, local) }
    catch (nativeError) { return await verifyRecoveryPublication(repo, release, main, assets, local) }
  } finally { rmSync(directory, { recursive: true, force: true }) }
}

function assetSnapshot(assets) {
  return new Map(assets.map((asset) => [asset.name, { id: asset.id, url: asset.url, digest: asset.digest, size: asset.size, uploader: asset.uploader?.id, state: asset.state }]))
}

function assertSameAssets(assets, snapshot, tag) {
  if (assets.length !== snapshot.size) throw new Error(`${tag}: release asset set changed during reconciliation`)
  for (const asset of assets) {
    const prior = snapshot.get(asset.name)
    if (!prior || asset.id !== prior.id || asset.url !== prior.url || asset.digest !== prior.digest || asset.size !== prior.size || asset.uploader?.id !== prior.uploader || asset.state !== prior.state) throw new Error(`${tag}: release asset identity changed during reconciliation`)
  }
}

async function verifyTrustedRelease(repo, summary, main) {
  if (!semver.test(summary?.tag_name || "") || !Number.isSafeInteger(summary?.id) || summary.id < 1 || summary.prerelease !== false || !bot(summary.author) || !sha40(summary.target_commitish)) return null
  const version = summary.tag_name.slice(1), source = summary.target_commitish
  if (versionTag(version) !== summary.tag_name) throw new Error("release tag canonicalization failed")
  const tagState = await readTagState(repo, summary.tag_name)
  if (tagState.commit !== source) throw new Error(`${summary.tag_name}: immutable tag does not match release source`)
  const release = await api(`repos/${repo}/releases/${summary.id}`)
  if (release.tag_name !== summary.tag_name || release.target_commitish !== source || release.prerelease !== false || !bot(release.author) || release.draft !== summary.draft || release.name !== summary.name) throw new Error(`${summary.tag_name}: release identity changed during reconciliation`)
  const assets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  assertAssetMetadata(assets, version, release.tag_name)
  const changelog = await sourceText(repo, source)
  const body = releaseBody(changelog, version, source)
  const legacy = legacyReleaseBody(changelog, version, source)
  if (!matchesTrustedReleaseBody(release.body, body, legacy)) throw new Error(`${release.tag_name}: current release body is not a recognized source-derived format`)
  await verifyPublicationProvenance(repo, release, version, source, main, assets)
  return { release, version, source, body, tagState, assets: assetSnapshot(assets) }
}

async function reconcile(repo, summary, main) {
  const trusted = await verifyTrustedRelease(repo, summary, main)
  if (!trusted) return false
  const { release, body } = trusted
  if (![true, false].includes(release.draft)) throw new Error(`${release.tag_name}: release has an invalid draft state`)
  if (release.body === body) return false

  const patched = await api(`repos/${repo}/releases/${release.id}`, { method: "PATCH", body: { body } })
  if (patched.id !== release.id || patched.tag_name !== release.tag_name || patched.target_commitish !== release.target_commitish || patched.draft !== release.draft || patched.prerelease !== release.prerelease || patched.body !== body || patched.name !== release.name || !bot(patched.author)) throw new Error(`${release.tag_name}: reconciled release metadata failed immediate verification`)

  const closingTag = await readTagState(repo, release.tag_name)
  if (!sameTagState(closingTag, trusted.tagState)) throw new Error(`${release.tag_name}: immutable tag changed during reconciliation`)
  const closing = await api(`repos/${repo}/releases/${release.id}`)
  if (closing.id !== release.id || closing.tag_name !== release.tag_name || closing.target_commitish !== release.target_commitish || closing.draft !== release.draft || closing.prerelease !== release.prerelease || closing.body !== body || closing.name !== release.name || !bot(closing.author)) throw new Error(`${release.tag_name}: release changed during closing verification`)
  if (!release.draft) {
    const byTag = await api(`repos/${repo}/releases/tags/${release.tag_name}`)
    if (byTag.id !== release.id || byTag.body !== body || byTag.target_commitish !== release.target_commitish || byTag.draft !== false) throw new Error(`${release.tag_name}: public tag lookup changed during reconciliation`)
  }
  const closingAssets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  assertSameAssets(closingAssets, trusted.assets, release.tag_name)
  assertAssetMetadata(closingAssets, trusted.version, release.tag_name)
  console.log(`${release.tag_name}: notes reconciled${release.draft ? " (draft preserved)" : ""}`)
  return true
}

async function run() {
  const repo = process.env.GITHUB_REPOSITORY || process.env.GH_REPO
  if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repo || "")) throw new Error("GITHUB_REPOSITORY is required")
  const expectedSHA = process.env.RECONCILE_EXECUTION_SHA || process.env.GITHUB_SHA
  const main = await executionMain(repo, expectedSHA)
  if ((process.argv[2] || "reconcile") === "verify-execution") {
    if (process.env.GITHUB_OUTPUT) await import("node:fs").then(({ appendFileSync }) => appendFileSync(process.env.GITHUB_OUTPUT, `sha=${main}\n`))
    console.log(`Verified reconciliation execution ${main}.`)
    return
  }
  const releases = await pages(`repos/${repo}/releases`)
  let changed = 0
  for (const release of releases.sort((a, b) => b.id - a.id)) if (await reconcile(repo, release, main)) changed += 1
  const closingMain = await executionMain(repo, expectedSHA)
  if (closingMain !== main) throw new Error("protected main changed during release reconciliation")
  console.log(`Release reconciliation complete; ${changed} release(s) updated.`)
}

run().catch((error) => { console.error(error.message); process.exitCode = 1 })
