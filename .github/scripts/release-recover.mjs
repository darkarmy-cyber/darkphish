import { appendFileSync, mkdtempSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from "node:fs"
import { createHash } from "node:crypto"
import { execFileSync } from "node:child_process"
import { tmpdir } from "node:os"
import { join } from "node:path"
import {
  api, assertCurrentVersionPublished, assetDisposition, expectedReleaseAssetNames, generatedPath,
  greenCommit, pages, peelTagToCommit, publicationReceiptName, repository, requiredChecks, verifyChecksums, versionTag,
} from "../../scripts/release-lib.mjs"
import { verifyCodeQLBaseline } from "../../scripts/codeql-baseline.mjs"
import { verifyReleaseMaintainerReview } from "../../scripts/release-maintainer-review.mjs"

const actionsBot = (actor) => actor?.login === "github-actions[bot]" && actor?.type === "Bot" && actor?.id === 41898282
const sha40 = (value) => typeof value === "string" && /^[a-f0-9]{40}$/.test(value)
const output = (name, value) => { if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, `${name}=${value}\n`) }
const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

function notesForVersion(changelog, version) {
  const start = changelog.indexOf(`## ${version} - `)
  if (start < 0) throw new Error("recovery source changelog is missing the release section")
  const end = changelog.indexOf("\n## ", start + 1)
  return changelog.slice(start, end < 0 ? undefined : end).trim()
}

function releaseName(version) {
  return `Darkphish ${version.split(".").slice(0, 2).join(".")}`
}

function releaseBody(notes, source) {
  return `${notes}\n\nSource commit: ${source}\n\n<!-- darkphish-release-source:${source} -->\n\nNative binaries, SHA-256 checksums and SPDX SBOM are attached.`
}

async function sourceText(repo, path, source) {
  const file = await api(`repos/${repo}/contents/${path}?ref=${source}`)
  if (file?.type !== "file" || file.encoding !== "base64" || typeof file.content !== "string") throw new Error(`recovery source ${path} is unavailable`)
  return Buffer.from(file.content.replace(/\n/g, ""), "base64").toString("utf8")
}

function validProtectedRepository(metadata, branch) {
  return metadata?.default_branch === "main" && metadata?.private === false && metadata?.fork === false && branch?.protected === true && sha40(branch?.commit?.sha)
}

async function currentProtectedMain(repo) {
  const metadata = await api(`repos/${repo}`)
  const branch = await api(`repos/${repo}/branches/main`)
  if (!validProtectedRepository(metadata, branch)) throw new Error("expected standalone public repository with protected main")
  const source = branch.commit.sha
  if (!await greenCommit(repo, source)) throw new Error("current protected main is not green")
  await verifyCodeQLBaseline(repo, source)

  const finalMetadata = await api(`repos/${repo}`)
  const finalBranch = await api(`repos/${repo}/branches/main`)
  if (!validProtectedRepository(finalMetadata, finalBranch) || finalBranch.commit.sha !== source) {
    throw new Error("protected main changed during CodeQL verification")
  }
  if (!await greenCommit(repo, source)) throw new Error("current protected main required checks changed during CodeQL verification")
  const closingBranch = await api(`repos/${repo}/branches/main`)
  if (!closingBranch?.protected || closingBranch.commit?.sha !== source) throw new Error("protected main changed after final required-check verification")
  return source
}

async function readTagState(repo, tag) {
  const ref = await api(`repos/${repo}/git/ref/tags/${tag}`, { missing: true })
  if (!ref) return null
  const objectType = ref.object?.type
  const objectSha = ref.object?.sha
  if (!sha40(objectSha) || !["commit", "tag"].includes(objectType)) throw new Error("release tag ref object is malformed")
  const commit = await peelTagToCommit(ref, (sha) => api(`repos/${repo}/git/tags/${sha}`))
  if (!sha40(commit)) throw new Error("release tag does not peel to a commit")
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

async function assertTagSnapshot(repo, tag, expected, message) {
  const current = await readTagState(repo, tag)
  if (expected === null) {
    if (current !== null) throw new Error(message)
    return null
  }
  if (!sameTagState(current, expected)) throw new Error(message)
  return current
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
  const files = await pages(`repos/${repo}/pulls/${pr.number}/files`)
  if (!files.length || files.some((file) => !generatedPath(file.filename))) throw new Error("pending release PR includes application changes")
  return pr
}

function successfulStep(job, name) {
  return job?.steps?.some((step) => step.name === name && step.status === "completed" && step.conclusion === "success")
}

async function verifyOriginalNativeRelease(repo, source) {
  const runs = await pages(`repos/${repo}/actions/runs?head_sha=${source}`, "workflow_runs")
  const ci = runs.filter((run) => run.name === "CI" && run.path === ".github/workflows/ci.yml" && run.event === "push" && run.head_branch === "main" && run.head_sha === source && run.status === "completed" && run.conclusion === "success")
  const codeql = runs.filter((run) => run.name === "CodeQL" && run.path === ".github/workflows/codeql.yml" && run.event === "push" && run.head_branch === "main" && run.head_sha === source && run.status === "completed" && run.conclusion === "success")
  if (!ci.length || !codeql.length) throw new Error("historical release source lacks successful exact-SHA CI or CodeQL")

  const candidates = []
  for (const run of runs.filter((item) => item.name === "Native release" && item.path === ".github/workflows/release.yml" && item.event === "workflow_run" && item.head_branch === "main" && item.head_sha === source && item.status === "completed" && item.conclusion === "failure")) {
    const jobs = await pages(`repos/${repo}/actions/runs/${run.id}/jobs`, "jobs")
    const metadata = jobs.find((job) => job.name === "metadata")
    const verify = jobs.find((job) => job.name === "verify")
    const smoke = jobs.find((job) => job.name === "audit-smoke")
    const publish = jobs.find((job) => job.name === "publish")
    const binaries = jobs.filter((job) => job.name?.startsWith("binaries ("))
    if (metadata?.conclusion !== "success" || verify?.conclusion !== "success" || smoke?.conclusion !== "success" || publish?.conclusion !== "failure" || binaries.length !== 5 || binaries.some((job) => job.conclusion !== "success")) continue
    if (!successfulStep(metadata, "Run node scripts/release-publish.mjs metadata") ||
        !successfulStep(publish, "Run actions/download-artifact@v8") ||
        !successfulStep(publish, "Run anchore/sbom-action@f8bdd1d8ac5e901a77a92f111440fdb1b593736b") ||
        !successfulStep(publish, "Generate checksums")) continue
    const failedPublish = publish.steps?.find((step) => step.name === "Publish verified assets without overwriting an existing release")
    if (failedPublish?.status !== "completed" || failedPublish.conclusion !== "failure") continue
    if (ci.every((item) => Date.parse(item.updated_at) > Date.parse(run.created_at)) || codeql.every((item) => Date.parse(item.updated_at) > Date.parse(run.created_at))) continue
    candidates.push(run)
  }
  if (!candidates.length) throw new Error("historical release provenance has no qualifying failed Native release run")
  const workflowIds = new Set(candidates.map((run) => run.workflow_id))
  if (workflowIds.size !== 1) throw new Error("historical release provenance spans unexpected Native release workflows")
  candidates.sort((a, b) => b.id - a.id)
  return candidates[0]
}

async function expectedMetadata(repo, source, version) {
  const changelog = await sourceText(repo, "CHANGELOG.md", source)
  const notes = notesForVersion(changelog, version)
  return { name: releaseName(version), body: releaseBody(notes, source) }
}

function assertExactDraft(release, version, source, expected) {
  const tag = versionTag(version)
  if (!release || release.tag_name !== tag || release.target_commitish !== source || release.name !== expected.name || release.body !== expected.body || release.draft !== true || release.prerelease !== false || release.published_at !== null || !actionsBot(release.author)) {
    throw new Error("pending release draft metadata does not exactly match the verified source")
  }
  return release
}

function assertExactPublished(release, version, source, expected) {
  const tag = versionTag(version)
  if (!release || release.tag_name !== tag || release.target_commitish !== source || release.name !== expected.name || release.body !== expected.body || release.draft !== false || release.prerelease !== false || !release.published_at || !actionsBot(release.author)) {
    throw new Error("published release metadata does not exactly match the verified source")
  }
  return release
}

function timestamp(value, label) {
  const parsed = Date.parse(value || "")
  if (!Number.isFinite(parsed)) throw new Error(`${label} timestamp is invalid`)
  return parsed
}

async function exactMainChecks(repo, source, before = Infinity) {
  const runs = await pages(`repos/${repo}/actions/runs?head_sha=${source}`, "workflow_runs")
  const successBefore = (run, name, path) => run.name === name && run.path === path && run.event === "push" && run.head_branch === "main" && run.head_sha === source && run.status === "completed" && run.conclusion === "success" && timestamp(run.updated_at, `${name} run`) <= before
  if (!runs.some((run) => successBefore(run, "CI", ".github/workflows/ci.yml")) || !runs.some((run) => successBefore(run, "CodeQL", ".github/workflows/codeql.yml"))) {
    throw new Error("recovery workflow source lacks successful exact-SHA CI or CodeQL before publication")
  }
}

async function exactRequiredChecksBefore(repo, source, before) {
  const checks = await pages(`repos/${repo}/commits/${source}/check-runs?filter=all`, "check_runs")
  for (const name of requiredChecks) {
    const qualifying = checks.some((check) =>
      check.name === name &&
      check.app?.slug === "github-actions" &&
      check.status === "completed" &&
      check.conclusion === "success" &&
      timestamp(check.completed_at, `${name} check`) <= before
    )
    if (!qualifying) throw new Error(`recovery workflow source lacks required successful check ${name} before publication`)
  }
}

async function downloadReleaseAsset(repo, asset, directory) {
  if (!Number.isSafeInteger(asset?.id) || asset.id < 1 || typeof asset?.name !== "string" || !/^(?:SHA256SUMS|darkphish-[A-Za-z0-9._-]+)$/.test(asset.name)) {
    throw new Error("published recovery asset metadata is malformed")
  }
  const url = new URL(asset.url || "")
  if (url.protocol !== "https:" || url.hostname !== "api.github.com" || url.pathname !== `/repos/${repo}/releases/assets/${asset.id}`) {
    throw new Error("published recovery asset API URL is unexpected")
  }
  const response = await fetch(url, {
    method: "GET",
    headers: { Accept: "application/octet-stream", Authorization: `Bearer ${process.env.GH_TOKEN || process.env.GITHUB_TOKEN}`, "X-GitHub-Api-Version": "2022-11-28" },
    redirect: "follow",
    signal: AbortSignal.timeout(30000),
  })
  if (!response.ok) throw new Error(`published recovery asset download failed with HTTP ${response.status}`)
  const bytes = Buffer.from(await response.arrayBuffer())
  const digest = createHash("sha256").update(bytes).digest("hex")
  if (asset.digest !== `sha256:${digest}` || asset.size !== bytes.length) throw new Error("published recovery asset bytes do not match GitHub digest metadata")
  const path = join(directory, asset.name)
  writeFileSync(path, bytes, { flag: "wx" })
  return { path, digest, size: bytes.length }
}

function verifyRecoveryAttestation(repo, path, recoverySHA) {
  if (!sha40(recoverySHA)) throw new Error("recovery attestation source SHA is malformed")
  execFileSync("gh", [
    "attestation", "verify", path,
    "--repo", repo,
    "--signer-workflow", `${repo}/.github/workflows/release-recover.yml`,
    "--signer-digest", recoverySHA,
    "--source-ref", "refs/heads/main",
    "--source-digest", recoverySHA,
    "--predicate-type", "https://slsa.dev/provenance/v1",
    "--deny-self-hosted-runners",
  ], {
    encoding: "utf8",
    env: { ...process.env, GH_TOKEN: process.env.GH_TOKEN || process.env.GITHUB_TOKEN },
    stdio: ["ignore", "pipe", "pipe"],
  })
}

async function qualifyingRecoveryRun(repo, release, version, main) {
  const receiptName = publicationReceiptName(version)
  const attestedNames = expectedReleaseAssetNames(version).filter((name) => name !== receiptName).sort()
  const assets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  const selected = assets.filter((asset) => attestedNames.includes(asset?.name))
  if (selected.length !== attestedNames.length || selected.map((asset) => asset.name).sort().join("\n") !== attestedNames.join("\n")) {
    throw new Error("published recovery release is missing attested base artifacts")
  }

  const directory = mkdtempSync(join(tmpdir(), "darkphish-recovery-attest-"))
  try {
    const local = new Map()
    for (const asset of selected) local.set(asset.name, await downloadReleaseAsset(repo, asset, directory))

    const publishedAt = timestamp(release.published_at, "release publication")
    const runs = await pages(`repos/${repo}/actions/workflows/release-recover.yml/runs?branch=main&status=success`, "workflow_runs")
    const candidates = runs.filter((run) =>
      run.name === "Recover pending release" &&
      run.path === ".github/workflows/release-recover.yml" &&
      run.head_branch === "main" &&
      sha40(run.head_sha) &&
      ["workflow_run", "schedule"].includes(run.event) &&
      run.status === "completed" &&
      run.conclusion === "success"
    ).sort((a, b) => b.id - a.id)

    for (const run of candidates) {
      try {
        const runStarted = timestamp(run.run_started_at || run.created_at, "recovery run start")
        const runFinished = timestamp(run.updated_at, "recovery run completion")
        if (publishedAt < runStarted || publishedAt > runFinished) continue
        await verifySourceAncestry(repo, run.head_sha, main)
        const recoveryCreated = timestamp(run.created_at, "recovery run creation")
        await exactMainChecks(repo, run.head_sha, recoveryCreated)
        await exactRequiredChecksBefore(repo, run.head_sha, recoveryCreated)

        const jobs = await pages(`repos/${repo}/actions/runs/${run.id}/jobs`, "jobs")
        const metadata = jobs.find((job) => job.name === "metadata")
        const verify = jobs.find((job) => job.name === "verify")
        const smoke = jobs.find((job) => job.name === "audit-smoke")
        const publish = jobs.find((job) => job.name === "publish")
        const binaries = jobs.filter((job) => job.name?.startsWith("binaries ("))
        if (metadata?.conclusion !== "success" || verify?.conclusion !== "success" || smoke?.conclusion !== "success" || publish?.conclusion !== "success" || binaries.length !== 5 || binaries.some((job) => job.conclusion !== "success")) continue
        if (!successfulStep(publish, "Attest rebuilt recovery artifacts") || !successfulStep(publish, "Publish rebuilt verified recovery assets")) continue
        const publishStep = publish.steps?.find((step) => step.name === "Publish rebuilt verified recovery assets")
        const publishStarted = timestamp(publishStep?.started_at, "recovery publish step start")
        const publishFinished = timestamp(publishStep?.completed_at, "recovery publish step completion")
        if (publishedAt < publishStarted || publishedAt > publishFinished) continue

        for (const name of attestedNames) verifyRecoveryAttestation(repo, local.get(name).path, run.head_sha)
        return { run, local }
      } catch (error) {
        console.warn(`Ignoring non-qualifying recovery run ${run.id}: ${error.message}`)
      }
    }
    throw new Error("published release has no qualifying cryptographically attested recovery run")
  } finally {
    rmSync(directory, { recursive: true, force: true })
  }
}

async function verifyPublishedRecovery(repo, release, version, main, tagState, expected) {
  const source = release.target_commitish
  assertExactPublished(release, version, source, expected)
  const canonical = await assertCurrentVersionPublished(repo, { version })
  if (canonical.id !== release.id) throw new Error("published recovery resolved a different release identity")
  await assertTagState(repo, versionTag(version), tagState, "published recovery tag ref object changed before attestation verification")

  const initialAssets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  const initialSnapshot = new Map(initialAssets.map((asset) => [asset.name, { digest: asset.digest, size: asset.size }]))
  const { run, local } = await qualifyingRecoveryRun(repo, release, version, main)

  const finalCanonical = await assertCurrentVersionPublished(repo, { version })
  assertExactPublished(finalCanonical, version, source, expected)
  if (finalCanonical.id !== release.id) throw new Error("published recovery release identity changed after attestation verification")
  await assertTagState(repo, versionTag(version), tagState, "published recovery tag ref object changed after attestation verification")
  const finalAssets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  if (finalAssets.length !== initialSnapshot.size) throw new Error("published recovery asset set changed during attestation verification")
  for (const asset of finalAssets) {
    const initial = initialSnapshot.get(asset.name)
    if (!initial || asset.digest !== initial.digest || asset.size !== initial.size || !actionsBot(asset.uploader) || asset.state !== "uploaded") throw new Error("published recovery asset state changed during attestation verification")
    const downloaded = local.get(asset.name)
    if (downloaded) assetDisposition(asset, downloaded.digest, downloaded.size)
  }
  const finalMain = await currentProtectedMain(repo)
  await verifySourceAncestry(repo, source, finalMain)
  await verifySourceAncestry(repo, run.head_sha, finalMain)
  return run
}

async function ensureDraftState(repo, releaseId, tag, expectedTagState = undefined) {
  let attempt = 0
  for (;;) {
    attempt += 1
    try {
      await api(`repos/${repo}/releases/${releaseId}`, { method: "PATCH", body: { draft: true, prerelease: false, make_latest: "false" } })
    } catch (error) {
      console.warn(`withdrawal PATCH attempt ${attempt} was ambiguous: ${error.message}`)
    }

    let confirmed = null
    try {
      const currentReleases = await pages(`repos/${repo}/releases`)
      const publicForTag = currentReleases.filter((release) => release?.tag_name === tag && release.draft === false)
      for (const publicRelease of publicForTag) {
        try {
          await api(`repos/${repo}/releases/${publicRelease.id}`, { method: "PATCH", body: { draft: true, prerelease: false, make_latest: "false" } })
        } catch (error) {
          console.warn(`replacement withdrawal PATCH attempt ${attempt} for release ${publicRelease.id} was ambiguous: ${error.message}`)
        }
      }

      const withdrawn = await api(`repos/${repo}/releases/${releaseId}`, { missing: true })
      const after = await pages(`repos/${repo}/releases`)
      const publicAfter = after.filter((release) => release?.tag_name === tag && release.draft === false)
      const byTag = await api(`repos/${repo}/releases/tags/${tag}`, { missing: true })
      const publicByTag = byTag && byTag.draft === false
      if (withdrawn?.draft === true && withdrawn?.prerelease === false && publicAfter.length === 0 && !publicByTag) confirmed = withdrawn
    } catch (error) {
      console.warn(`withdrawal probe attempt ${attempt} failed: ${error.message}`)
    }

    if (confirmed) {
      if (expectedTagState !== undefined) await assertTagSnapshot(repo, tag, expectedTagState, "immutable release tag ref object changed while withdrawing publication")
      return confirmed
    }
    await delay(Math.min(5000, 500 * attempt))
  }
}

async function withdrawPublishedRelease(repo, releaseId, tag, expectedTagState, originalError) {
  const withdrawn = await ensureDraftState(repo, releaseId, tag, expectedTagState)
  if (withdrawn?.draft !== true) throw new Error("CRITICAL: release withdrawal returned without confirmed draft state")
  throw new Error(`release publication was withdrawn after verification failed: ${originalError.message}`)
}

async function recoveryState(repo, version, main) {
  const tag = versionTag(version)
  let releases = await pages(`repos/${repo}/releases`)
  let tagged = releases.filter((release) => release?.tag_name === tag)
  let published = tagged.filter((release) => release.draft === false)
  const initialTagState = await readTagState(repo, tag)

  if (published.length === 1 && initialTagState && sha40(published[0]?.target_commitish) && initialTagState.commit === published[0].target_commitish) {
    const candidate = published[0]
    try {
      const source = candidate.target_commitish
      await verifySourceAncestry(repo, source, main)
      await verifiedReleasePR(repo, source, version, tag)
      const originalRun = await verifyOriginalNativeRelease(repo, source)
      const expected = await expectedMetadata(repo, source, version)
      const recoveryRun = await verifyPublishedRecovery(repo, candidate, version, main, initialTagState, expected)
      const commit = await api(`repos/${repo}/commits/${source}`)
      const builtAt = commit?.commit?.committer?.date
      if (!builtAt || Number.isNaN(Date.parse(builtAt))) throw new Error("published recovery source commit timestamp is unavailable")
      return { tag, source, published: candidate, recoveryRun, originalRun, expected, builtAt: new Date(builtAt).toISOString(), tagState: initialTagState }
    } catch (error) {
      console.warn(`Existing public ${tag} is not a fully attested recovery publication and will be withdrawn: ${error.message}`)
    }
  }

  if (published.length) {
    for (const release of published) await ensureDraftState(repo, release.id, tag, initialTagState)
    releases = await pages(`repos/${repo}/releases`)
    tagged = releases.filter((release) => release?.tag_name === tag)
    published = tagged.filter((release) => release.draft === false)
    if (published.length) throw new Error("published release state could not be withdrawn before trusted recovery")
    console.warn(`Withdrew unverified public ${tag}; recovery will rebuild and attest all assets before publication.`)
  }

  const tagState = await assertTagSnapshot(repo, tag, initialTagState, "release tag ref object changed during recovery state verification")
  const drafts = tagged.filter((release) => release.draft === true)
  if (drafts.length > 1) throw new Error("multiple pending release drafts require manual investigation")

  const tagSHA = tagState?.commit || null
  const source = drafts[0]?.target_commitish || tagSHA
  if (!source) return null
  if (!sha40(source)) throw new Error("pending release source is malformed")
  if (tagSHA && tagSHA !== source) throw new Error("release tag already exists at an unexpected commit; tags are immutable")

  await verifySourceAncestry(repo, source, main)
  await verifiedReleasePR(repo, source, version, tag)
  const originalRun = await verifyOriginalNativeRelease(repo, source)
  const expected = await expectedMetadata(repo, source, version)
  if (drafts[0]) assertExactDraft(drafts[0], version, source, expected)

  const commit = await api(`repos/${repo}/commits/${source}`)
  const builtAt = commit?.commit?.committer?.date
  if (!builtAt || Number.isNaN(Date.parse(builtAt))) throw new Error("pending release source commit timestamp is unavailable")
  return { tag, source, draft: drafts[0] || null, originalRun, expected, builtAt: new Date(builtAt).toISOString(), tagState }
}

function localArtifacts(version) {
  const receiptName = publicationReceiptName(version)
  if (!receiptName) throw new Error("release recovery requires publication receipt support")
  const expected = expectedReleaseAssetNames(version).filter((name) => name !== receiptName).sort()
  const names = readdirSync("dist").filter((name) => statSync(`dist/${name}`).isFile()).sort()
  if (names.join("\n") !== expected.join("\n")) throw new Error("rebuilt recovery artifact set is incomplete or unexpected")
  const bytes = new Map(names.map((name) => [name, readFileSync(`dist/${name}`)]))
  const hashes = new Map([...bytes].map(([name, value]) => [name, createHash("sha256").update(value).digest("hex")]))
  verifyChecksums(bytes.get("SHA256SUMS").toString("utf8"), hashes)
  return { receiptName, names, bytes, hashes }
}

async function uploadAsset(release, name, content) {
  const url = new URL(release.upload_url.split("{")[0])
  if (url.origin !== "https://uploads.github.com") throw new Error("unexpected release upload host")
  url.searchParams.set("name", name)
  const response = await fetch(url, {
    method: "POST",
    headers: { Authorization: `Bearer ${process.env.GH_TOKEN || process.env.GITHUB_TOKEN}`, "Content-Type": "application/octet-stream" },
    body: content,
    redirect: "error",
    signal: AbortSignal.timeout(30000),
  })
  if (!response.ok) throw new Error(`release recovery asset upload failed with HTTP ${response.status}`)
}

function assertUploadedAssetSet(assets, local, receiptHash, receiptLength) {
  const expected = [...local.names, local.receiptName].sort()
  const names = assets.map((asset) => asset?.name).sort()
  if (assets.length !== expected.length || new Set(names).size !== names.length || names.join("\n") !== expected.join("\n")) throw new Error("recovery release asset set changed or is incomplete")
  for (const asset of assets) {
    if (!actionsBot(asset?.uploader) || asset.state !== "uploaded") throw new Error("recovery release contains an asset from an untrusted uploader")
    if (asset.name === local.receiptName) assetDisposition(asset, receiptHash, receiptLength)
    else assetDisposition(asset, local.hashes.get(asset.name), local.bytes.get(asset.name)?.length)
  }
}

async function ensureTag(repo, tag, source, expectedInitialState) {
  let current = await readTagState(repo, tag)
  if (expectedInitialState?.present === true) {
    const expected = { objectType: expectedInitialState.objectType, objectSha: expectedInitialState.objectSha, commit: source }
    if (!sameTagState(current, expected)) throw new Error("release tag ref object changed since recovery metadata verification")
  } else if (expectedInitialState?.present === false) {
    if (current) throw new Error("release tag appeared since recovery metadata verification")
    await api(`repos/${repo}/git/refs`, { method: "POST", body: { ref: `refs/tags/${tag}`, sha: source } })
    current = await readTagState(repo, tag)
  } else if (!current) {
    await api(`repos/${repo}/git/refs`, { method: "POST", body: { ref: `refs/tags/${tag}`, sha: source } })
    current = await readTagState(repo, tag)
  }
  if (!current || current.commit !== source) throw new Error("release tag does not resolve to the immutable verified source")
  return current
}

async function metadata() {
  const repo = repository()
  const version = readFileSync("VERSION", "utf8").trim()
  versionTag(version)
  const main = await currentProtectedMain(repo)
  const state = await recoveryState(repo, version, main)
  if (!state) {
    output("ready", "false")
    console.log("No pending release recovery is required.")
    return
  }
  if (state.published) {
    output("ready", "false")
    console.log(`Verified existing attested recovery publication ${state.tag} from recovery run ${state.recoveryRun.id}; no recovery action is required.`)
    return
  }

  const finalMain = await currentProtectedMain(repo)
  if (finalMain !== main) throw new Error("protected main changed during recovery metadata verification")
  await verifySourceAncestry(repo, state.source, finalMain)

  output("ready", "true")
  output("source", state.source)
  output("main", finalMain)
  output("version", version)
  output("tag", state.tag)
  output("built_at", state.builtAt)
  output("original_run_id", state.originalRun.id)
  output("tag_present", state.tagState ? "true" : "false")
  output("tag_object_type", state.tagState?.objectType || "")
  output("tag_object_sha", state.tagState?.objectSha || "")
  console.log(`Verified recovery source ${state.source} from Native release run ${state.originalRun.id}`)
}

async function publish() {
  const repo = repository()
  const version = process.env.RECOVERY_VERSION
  const source = process.env.RECOVERY_SOURCE
  const expectedMain = process.env.RECOVERY_MAIN
  if (!version || !sha40(source) || !sha40(expectedMain)) throw new Error("recovery publication inputs are invalid")
  const tag = versionTag(version)

  const tagPresentValue = process.env.RECOVERY_TAG_PRESENT
  if (!["true", "false"].includes(tagPresentValue)) throw new Error("recovery tag snapshot input is invalid")
  const initialTagExpectation = { present: tagPresentValue === "true" }
  if (initialTagExpectation.present) {
    initialTagExpectation.objectType = process.env.RECOVERY_TAG_OBJECT_TYPE
    initialTagExpectation.objectSha = process.env.RECOVERY_TAG_OBJECT_SHA
    if (!["commit", "tag"].includes(initialTagExpectation.objectType) || !sha40(initialTagExpectation.objectSha)) throw new Error("recovery tag object snapshot is invalid")
  }

  const checkout = process.env.RECOVERY_SOURCE_DIR || ".cache/source"
  const checkedOut = execFileSync("git", ["-C", checkout, "rev-parse", "HEAD"], { encoding: "utf8" }).trim()
  if (checkedOut !== source || readFileSync(`${checkout}/VERSION`, "utf8").trim() !== version) throw new Error("recovery build checkout is not the immutable release source")

  const local = localArtifacts(version)
  const receipt = Buffer.from(`${JSON.stringify({
    schema: "darkphish-release-publication-receipt/v1",
    tag,
    source_sha: source,
    checksums_sha256: local.hashes.get("SHA256SUMS"),
  })}\n`, "utf8")
  const receiptHash = createHash("sha256").update(receipt).digest("hex")

  let main = await currentProtectedMain(repo)
  if (main !== expectedMain) throw new Error("protected main changed since recovery metadata verification")
  let state = await recoveryState(repo, version, main)
  if (!state || state.source !== source || state.originalRun.id.toString() !== process.env.RECOVERY_ORIGINAL_RUN_ID) throw new Error("release recovery provenance changed before publication")

  const immutableTag = await ensureTag(repo, tag, source, initialTagExpectation)
  main = await currentProtectedMain(repo)
  if (main !== expectedMain) throw new Error("protected main changed after recovery tag creation")
  await verifySourceAncestry(repo, source, main)
  await verifiedReleasePR(repo, source, version, tag)
  const originalRun = await verifyOriginalNativeRelease(repo, source)
  if (originalRun.id.toString() !== process.env.RECOVERY_ORIGINAL_RUN_ID) throw new Error("selected historical Native release provenance changed")
  await assertTagState(repo, tag, immutableTag, "immutable release tag ref object changed after recovery tag creation")

  if (state.draft) {
    assertExactDraft(await api(`repos/${repo}/releases/${state.draft.id}`), version, source, state.expected)
    await api(`repos/${repo}/releases/${state.draft.id}`, { method: "DELETE" })
    if (await api(`repos/${repo}/releases/${state.draft.id}`, { missing: true })) throw new Error("stale release draft still exists after deletion")
  }
  const remaining = (await pages(`repos/${repo}/releases`)).filter((release) => release?.tag_name === tag)
  if (remaining.length) throw new Error("release state appeared after stale draft deletion")

  let release = await api(`repos/${repo}/releases`, { method: "POST", body: {
    tag_name: tag,
    target_commitish: source,
    name: state.expected.name,
    body: state.expected.body,
    draft: true,
    prerelease: false,
    make_latest: "true",
  } })
  assertExactDraft(release, version, source, state.expected)

  for (const name of local.names) await uploadAsset(release, name, local.bytes.get(name))
  await uploadAsset(release, local.receiptName, receipt)

  release = assertExactDraft(await api(`repos/${repo}/releases/${release.id}`), version, source, state.expected)
  let assets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  assertUploadedAssetSet(assets, local, receiptHash, receipt.length)
  await assertTagState(repo, tag, immutableTag, "release tag ref object changed before publication")

  main = await currentProtectedMain(repo)
  if (main !== expectedMain) throw new Error("protected main changed immediately before recovery publication")
  await verifySourceAncestry(repo, source, main)
  await verifiedReleasePR(repo, source, version, tag)
  const finalOriginalRun = await verifyOriginalNativeRelease(repo, source)
  if (finalOriginalRun.id.toString() !== process.env.RECOVERY_ORIGINAL_RUN_ID) throw new Error("historical Native release provenance changed immediately before publication")
  release = assertExactDraft(await api(`repos/${repo}/releases/${release.id}`), version, source, state.expected)
  assets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  assertUploadedAssetSet(assets, local, receiptHash, receipt.length)
  await assertTagState(repo, tag, immutableTag, "release tag ref object changed immediately before publication")

  try {
    await api(`repos/${repo}/releases/${release.id}`, { method: "PATCH", body: { draft: false, prerelease: false, make_latest: "true" } })
  } catch (error) {
    await withdrawPublishedRelease(repo, release.id, tag, immutableTag, new Error(`publication PATCH returned an ambiguous failure: ${error.message}`))
  }

  try {
    const postMain = await currentProtectedMain(repo)
    if (postMain !== expectedMain) throw new Error("protected main changed during the publication window")
    await verifySourceAncestry(repo, source, postMain)
    await verifiedReleasePR(repo, source, version, tag)
    const postOriginalRun = await verifyOriginalNativeRelease(repo, source)
    if (postOriginalRun.id.toString() !== process.env.RECOVERY_ORIGINAL_RUN_ID) throw new Error("historical Native release provenance changed during publication")

    const published = assertExactPublished(await api(`repos/${repo}/releases/${release.id}`), version, source, state.expected)
    const byTag = assertExactPublished(await api(`repos/${repo}/releases/tags/${tag}`), version, source, state.expected)
    if (published.id !== release.id || byTag.id !== release.id) throw new Error("post-publication release identity verification failed")
    await assertTagState(repo, tag, immutableTag, "post-publication tag ref object verification failed")
    const publishedAssets = await pages(`repos/${repo}/releases/${release.id}/assets`)
    assertUploadedAssetSet(publishedAssets, local, receiptHash, receipt.length)

    const fullyVerified = await assertCurrentVersionPublished(repo, { version })
    assertExactPublished(fullyVerified, version, source, state.expected)
    if (fullyVerified.id !== release.id) throw new Error("published release verification resolved a different release")

    const finalRelease = assertExactPublished(await api(`repos/${repo}/releases/${release.id}`), version, source, state.expected)
    const finalByTag = assertExactPublished(await api(`repos/${repo}/releases/tags/${tag}`), version, source, state.expected)
    if (finalRelease.id !== release.id || finalByTag.id !== release.id) throw new Error("final published release identity verification failed")
    const finalAssets = await pages(`repos/${repo}/releases/${release.id}/assets`)
    assertUploadedAssetSet(finalAssets, local, receiptHash, receipt.length)
    await assertTagState(repo, tag, immutableTag, "final immutable release tag ref object verification failed")
    const finalMain = await currentProtectedMain(repo)
    if (finalMain !== expectedMain) throw new Error("protected main changed before final recovery publication acceptance")
    await verifySourceAncestry(repo, source, finalMain)
  } catch (error) {
    await withdrawPublishedRelease(repo, release.id, tag, immutableTag, error)
  }

  console.log(`Rebuilt, published and verified https://github.com/${repo}/releases/tag/${tag} from immutable source ${source}`)
}

const command = process.argv[2] || "metadata"
const runner = command === "metadata" ? metadata : command === "publish" ? publish : null
if (!runner) throw new Error(`unknown recovery command ${command}`)
runner().catch((error) => { console.error(error.message); process.exitCode = 1 })
