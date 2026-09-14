import { pathToFileURL } from "node:url"
import { appendFileSync, readFileSync } from "node:fs"
import { setTimeout as pause } from "node:timers/promises"
import { api } from "./release-lib.mjs"
import { executionMain, verifyTrustedRelease } from "./release-reconcile.mjs"
import { verifyPullRequestReviews } from "./review-gate.mjs"
import { normalizationHold } from "./release-normalization-hold.mjs"

const repo = "darkarmy-cyber/darkphish", releaseID = 388329244
const source = "73bf5948ed19cd918638453d478ef3f1cbe83d89"
const manifest = "sha256:d41dea173bb71a7230b26bd10b60209285a9e9892a07e7267dd8e46c63a8e7b7"
const originalPublishedAt = "2026-09-14T11:01:32Z"
const actor = user => user?.login === "github-actions[bot]" && user.id === 41898282 && user.type === "Bot"
function assertIdentity(release) {
  if (release?.id !== releaseID || release.tag_name !== "v0.7.1" || release.target_commitish !== source ||
    release.name !== "Darkphish 0.7" || release.prerelease !== false || !actor(release.author) ||
    release.published_at !== originalPublishedAt) throw new Error("Resumption release identity or historical publication time changed")
}
function assertManifest(trusted) {
  if (!trusted || trusted.assets?.size !== 8 || trusted.assets.get("SHA256SUMS")?.digest !== manifest) throw new Error("Resumption artifact set changed")
}

export function rebuildRestricted(version) {
  if (!/^\d+\.\d+\.\d+$/.test(version)) throw new Error("Invalid rebuild VERSION")
  return version === "0.7.1"
}

async function withdrawRelease(request, delay) {
  for (let attempt = 0; attempt < 5; attempt++) {
    try { await request(`repos/${repo}/releases/${releaseID}`, { method: "PATCH", body: { draft: true, prerelease: false, make_latest: "false" } }) } catch {}
    try {
      const direct = await request(`repos/${repo}/releases/${releaseID}`)
      const byTag = await request(`repos/${repo}/releases/tags/v0.7.1`, { missing: true })
      const releases = await request(`repos/${repo}/releases?per_page=100`)
      if (direct?.id === releaseID && direct.draft === true && byTag?.draft !== false && Array.isArray(releases) &&
        releases.some(r => r.id === releaseID && r.draft === true) && !releases.some(r => r.tag_name === "v0.7.1" && r.draft === false)) return
    } catch {}
    if (attempt < 4) await delay(250 * (attempt + 1))
  }
  throw new Error("CRITICAL: could not confirm failed resumption withdrawal through direct, collection and tag lookups")
}

export async function resumeRelease(main, { request = api, execution = executionMain, verify = verifyTrustedRelease,
  reviews = verifyPullRequestReviews, hold = normalizationHold, log = console.log, delay = pause } = {}) {
  const candidate = await request(`repos/${repo}/pulls/58`)
  if (candidate?.number !== 58 || !/^[a-f0-9]{40}$/.test(main || "")) throw new Error("Invalid resumption target")
  if (candidate.state !== "closed" || !candidate.merged_at || candidate.merge_commit_sha !== main) {
    log("Not the one-shot PR58 merge; no resumption mutation."); return
  }
  let release = await request(`repos/${repo}/releases/${releaseID}`)
  let mayBePublic = release?.draft === false
  try {
    if (hold()) throw new Error("Release normalization hold remains active")
    await execution(repo, main)
    const checkAuthorization = async () => {
      const pr = await request(`repos/${repo}/pulls/58`)
      if (pr.number !== 58 || pr.state !== "closed" || !pr.merged_at || pr.merge_commit_sha !== main || pr.base?.ref !== "main" ||
        pr.head?.repo?.full_name !== repo || pr.head.ref !== "codex/threads/019fb3b4-63f2-7180-8a29-babee7e6a51b/release071-finalize") {
        throw new Error("Current main is not the reviewed resumption PR58 merge")
      }
      await reviews(repo, pr, { get: request, query: body => request("graphql", { method: "POST", body }) })
    }
    await checkAuthorization()
    assertIdentity(release)
    const trusted = await verify(repo, release, main)
    assertManifest(trusted)
    const checkTag = async () => {
      const ref = await request(`repos/${repo}/git/ref/tags/v0.7.1`)
      if (!trusted.tagState || ref?.object?.type !== trusted.tagState.objectType || ref.object.sha !== trusted.tagState.objectSha) {
        throw new Error("Immutable tag object changed during resumption")
      }
    }
    if (release.body !== trusted.body) throw new Error("Source-derived release notes must be reconciled before resumption")
    if (release.draft === false) { log("Verified v0.7.1 is already public; no mutation."); return }
    if (release.draft !== true) throw new Error("Unexpected resumption draft state")
    await execution(repo, main)
    await checkAuthorization()
    const closing = await request(`repos/${repo}/releases/${releaseID}`)
    if (closing?.draft === false) mayBePublic = true
    assertIdentity(closing)
    if (closing.draft !== true || closing.body !== release.body) throw new Error("Draft changed before resumption")
    const assets = await request(`repos/${repo}/releases/${releaseID}/assets?per_page=100`)
    if (!Array.isArray(assets) || assets.length !== trusted.assets.size || new Set(assets.map(a => a.name)).size !== assets.length || assets.some(a => {
      const old = trusted.assets.get(a.name)
      return !old || a.id !== old.id || a.digest !== old.digest || a.size !== old.size || a.state !== "uploaded" || !actor(a.uploader)
    })) throw new Error("Draft assets changed before resumption")
    await checkTag()
    mayBePublic = true // A transport error does not prove the publish PATCH failed.
    await request(`repos/${repo}/releases/${releaseID}`, { method: "PATCH", body: { draft: false, prerelease: false, make_latest: "true" } })
    release = await request(`repos/${repo}/releases/${releaseID}`)
    assertIdentity(release)
    if (release.draft !== false || release.body !== trusted.body) throw new Error("Resumed publication metadata changed")
    const byTag = await request(`repos/${repo}/releases/tags/v0.7.1`)
    assertIdentity(byTag)
    if (byTag.draft !== false || byTag.body !== trusted.body) throw new Error("Public tag lookup disagrees after resumption")
    assertManifest(await verify(repo, release, main))
    await execution(repo, main)
    await checkAuthorization()
    await checkTag()
    const finalRelease = await request(`repos/${repo}/releases/${releaseID}`)
    assertIdentity(finalRelease)
    if (finalRelease.draft !== false || finalRelease.body !== trusted.body) throw new Error("Release changed during final resumption checks")
    const finalAssets = await request(`repos/${repo}/releases/${releaseID}/assets?per_page=100`)
    if (!Array.isArray(finalAssets) || finalAssets.length !== assets.length || new Set(finalAssets.map(a => a.name)).size !== assets.length ||
      finalAssets.some(a => {
        const original = trusted.assets.get(a.name)
        return !original || a.id !== original.id || a.digest !== original.digest || a.size !== original.size || a.state !== "uploaded" || !actor(a.uploader)
      })) throw new Error("Release assets changed during final resumption checks")
    log(`Verified resumed v0.7.1 release ${releaseID}; original artifacts and publication provenance preserved.`)
  } catch (error) {
    // The write may have succeeded despite a transport error. Withdraw only the
    // precisely authorized release; never delete or replace any artifact.
    if (mayBePublic) await withdrawRelease(request, delay)
    throw error
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  if (process.argv[2] === "guard-rebuild") {
    const restricted = rebuildRestricted(readFileSync(new URL("../VERSION", import.meta.url), "utf8").trim())
    const active = Boolean(normalizationHold()) || restricted
    if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, `active=${active}\n`)
    console.log(active ? "Generic publication is held; v0.7.1 must reuse the selected original artifacts." : "No generic publication hold.")
  } else {
    if (process.env.GITHUB_REPOSITORY !== repo) throw new Error("Unexpected resumption repository")
    resumeRelease(process.env.RESUME_EXECUTION_SHA).catch(error => { console.error(error.message); process.exitCode = 1 })
  }
}
