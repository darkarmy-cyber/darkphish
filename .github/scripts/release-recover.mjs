import { appendFileSync, readFileSync, readdirSync, statSync } from "node:fs"
import { createHash } from "node:crypto"
import { execFileSync } from "node:child_process"
import {
  api, assertCurrentVersionPublished, assetDisposition, expectedReleaseAssetNames, generatedPath,
  greenCommit, pages, peelTagToCommit, publicationReceiptName, repository, verifyChecksums, versionTag,
} from "../../scripts/release-lib.mjs"
import { verifyCodeQLBaseline } from "../../scripts/codeql-baseline.mjs"
import { verifyReleaseMaintainerReview } from "../../scripts/release-maintainer-review.mjs"

const actionsBot = (actor) => actor?.login === "github-actions[bot]" && actor?.type === "Bot" && actor?.id === 41898282
const sha40 = (value) => typeof value === "string" && /^[a-f0-9]{40}$/.test(value)
const output = (name, value) => { if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, `${name}=${value}\n`) }

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

async function currentProtectedMain(repo) {
  const metadata = await api(`repos/${repo}`)
  const branch = await api(`repos/${repo}/branches/main`)
  if (metadata.default_branch !== "main" || metadata.private !== false || metadata.fork !== false || !branch.protected || !sha40(branch.commit?.sha)) {
    throw new Error("expected standalone public repository with protected main")
  }
  if (!await greenCommit(repo, branch.commit.sha)) throw new Error("current protected main is not green")
  await verifyCodeQLBaseline(repo, branch.commit.sha)
  return branch.commit.sha
}

async function tagCommit(repo, tag) {
  const ref = await api(`repos/${repo}/git/ref/tags/${tag}`, { missing: true })
  if (!ref) return null
  return peelTagToCommit(ref, (sha) => api(`repos/${repo}/git/tags/${sha}`))
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
  const ci = runs.filter((run) => run.name === "CI" && run.event === "push" && run.head_branch === "main" && run.head_sha === source && run.conclusion === "success")
  const codeql = runs.filter((run) => run.name === "CodeQL" && run.event === "push" && run.head_branch === "main" && run.head_sha === source && run.conclusion === "success")
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

async function recoveryState(repo, version, main) {
  const tag = versionTag(version)
  const releases = await pages(`repos/${repo}/releases`)
  const tagged = releases.filter((release) => release?.tag_name === tag)
  const published = tagged.filter((release) => release.draft === false)
  if (published.length) {
    if (published.length !== 1 || tagged.length !== 1) throw new Error("release tag maps to ambiguous published state")
    await assertCurrentVersionPublished(repo, { version })
    return { published: true, tag }
  }
  const drafts = tagged.filter((release) => release.draft === true)
  if (drafts.length > 1) throw new Error("multiple pending release drafts require manual investigation")

  const tagSHA = await tagCommit(repo, tag)
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
  return { published: false, tag, source, draft: drafts[0] || null, originalRun, expected, builtAt: new Date(builtAt).toISOString() }
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

async function ensureTag(repo, tag, source) {
  let current = await tagCommit(repo, tag)
  if (!current) {
    await api(`repos/${repo}/git/refs`, { method: "POST", body: { ref: `refs/tags/${tag}`, sha: source } })
    current = await tagCommit(repo, tag)
  }
  if (current !== source) throw new Error("release tag does not resolve to the immutable verified source")
}

async function withdrawPublishedRelease(repo, releaseId, tag, source, originalError) {
  try {
    await api(`repos/${repo}/releases/${releaseId}`, { method: "PATCH", body: { draft: true, prerelease: false, make_latest: "false" } })
    const withdrawn = await api(`repos/${repo}/releases/${releaseId}`)
    if (withdrawn?.draft !== true || withdrawn?.prerelease !== false) throw new Error("withdrawn release did not return to draft state")
    if (await tagCommit(repo, tag) !== source) throw new Error("immutable release tag changed while withdrawing publication")
  } catch (withdrawError) {
    throw new Error(`CRITICAL: post-publication verification failed and release withdrawal also failed: ${originalError.message}; withdrawal: ${withdrawError.message}`)
  }
  throw new Error(`release publication was withdrawn after post-publication verification failed: ${originalError.message}`)
}

async function metadata() {
  const repo = repository()
  const version = readFileSync("VERSION", "utf8").trim()
  versionTag(version)
  const main = await currentProtectedMain(repo)
  const state = await recoveryState(repo, version, main)
  if (!state || state.published) {
    output("ready", "false")
    console.log(state?.published ? `Release ${state.tag} is already fully published and verified.` : "No pending release recovery is required.")
    return
  }
  output("ready", "true")
  output("source", state.source)
  output("main", main)
  output("version", version)
  output("tag", state.tag)
  output("built_at", state.builtAt)
  output("original_run_id", state.originalRun.id)
  console.log(`Verified recovery source ${state.source} from Native release run ${state.originalRun.id}`)
}

async function publish() {
  const repo = repository()
  const version = process.env.RECOVERY_VERSION
  const source = process.env.RECOVERY_SOURCE
  const expectedMain = process.env.RECOVERY_MAIN
  if (!version || !sha40(source) || !sha40(expectedMain)) throw new Error("recovery publication inputs are invalid")
  const tag = versionTag(version)

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
  if (!state || state.published || state.source !== source || state.originalRun.id.toString() !== process.env.RECOVERY_ORIGINAL_RUN_ID) throw new Error("release recovery provenance changed before publication")

  await ensureTag(repo, tag, source)
  main = await currentProtectedMain(repo)
  if (main !== expectedMain) throw new Error("protected main changed after recovery tag creation")
  await verifySourceAncestry(repo, source, main)
  await verifiedReleasePR(repo, source, version, tag)
  const originalRun = await verifyOriginalNativeRelease(repo, source)
  if (originalRun.id.toString() !== process.env.RECOVERY_ORIGINAL_RUN_ID) throw new Error("selected historical Native release provenance changed")

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
  if (await tagCommit(repo, tag) !== source) throw new Error("release tag changed before publication")

  main = await currentProtectedMain(repo)
  if (main !== expectedMain) throw new Error("protected main changed immediately before recovery publication")
  await verifySourceAncestry(repo, source, main)
  await verifiedReleasePR(repo, source, version, tag)
  const finalOriginalRun = await verifyOriginalNativeRelease(repo, source)
  if (finalOriginalRun.id.toString() !== process.env.RECOVERY_ORIGINAL_RUN_ID) throw new Error("historical Native release provenance changed immediately before publication")
  release = assertExactDraft(await api(`repos/${repo}/releases/${release.id}`), version, source, state.expected)
  assets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  assertUploadedAssetSet(assets, local, receiptHash, receipt.length)
  if (await tagCommit(repo, tag) !== source) throw new Error("release tag changed immediately before publication")

  await api(`repos/${repo}/releases/${release.id}`, { method: "PATCH", body: { draft: false, prerelease: false, make_latest: "true" } })

  try {
    const postMain = await currentProtectedMain(repo)
    if (postMain !== expectedMain) throw new Error("protected main changed during the publication window")
    await verifySourceAncestry(repo, source, postMain)
    await verifiedReleasePR(repo, source, version, tag)
    const postOriginalRun = await verifyOriginalNativeRelease(repo, source)
    if (postOriginalRun.id.toString() !== process.env.RECOVERY_ORIGINAL_RUN_ID) throw new Error("historical Native release provenance changed during publication")

    const published = await api(`repos/${repo}/releases/${release.id}`)
    const byTag = await api(`repos/${repo}/releases/tags/${tag}`)
    if (published.id !== release.id || byTag.id !== release.id || published.draft !== false || byTag.draft !== false || published.target_commitish !== source || byTag.target_commitish !== source || published.name !== state.expected.name || published.body !== state.expected.body || !actionsBot(published.author) || !actionsBot(byTag.author)) {
      throw new Error("post-publication release metadata verification failed")
    }
    if (await tagCommit(repo, tag) !== source) throw new Error("post-publication tag verification failed")
    const publishedAssets = await pages(`repos/${repo}/releases/${release.id}/assets`)
    assertUploadedAssetSet(publishedAssets, local, receiptHash, receipt.length)
    const fullyVerified = await assertCurrentVersionPublished(repo, { version })
    if (fullyVerified.id !== release.id) throw new Error("published release verification resolved a different release")
  } catch (error) {
    await withdrawPublishedRelease(repo, release.id, tag, source, error)
  }

  console.log(`Rebuilt, published and verified https://github.com/${repo}/releases/tag/${tag} from immutable source ${source}`)
}

const command = process.argv[2] || "metadata"
const runner = command === "metadata" ? metadata : command === "publish" ? publish : null
if (!runner) throw new Error(`unknown recovery command ${command}`)
runner().catch((error) => { console.error(error.message); process.exitCode = 1 })
