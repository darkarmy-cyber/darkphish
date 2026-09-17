import { appendFileSync, readFileSync } from "node:fs"
import { execFileSync } from "node:child_process"
import { fileURLToPath } from "node:url"
import { api, pages, repository, versionTag } from "../../scripts/release-lib.mjs"

const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const output = (name, value) => { if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, `${name}=${value}\n`) }
const sha40 = (value) => typeof value === "string" && /^[a-f0-9]{40}$/.test(value)

async function currentTag(repo, tag) {
  const ref = await api(`repos/${repo}/git/ref/tags/${tag}`, { missing: true })
  if (!ref) return null
  const objectType = ref.object?.type
  const objectSha = ref.object?.sha
  if (!["commit", "tag"].includes(objectType) || !sha40(objectSha)) throw new Error("release tag ref object is malformed")
  return { objectType, objectSha }
}

function encodedTagSnapshot(tagState) {
  return tagState ? `false:${tagState.objectType}:${tagState.objectSha}` : "true"
}

async function publicReleases(repo, tag) {
  return (await pages(`repos/${repo}/releases`)).filter((release) => release?.tag_name === tag && release.draft === false)
}

async function directSnapshot(repo, ids) {
  return Promise.all([...ids].map((id) => api(`repos/${repo}/releases/${id}`, { missing: true })))
}

async function withdrawWhileTagAbsent(repo, tag, releases, executionSHA) {
  let tagAppeared = false
  const ids = new Set(releases.filter((release) => Number.isSafeInteger(release?.id)).map((release) => release.id))
  for (let attempt = 1; ; attempt += 1) {
    for (const release of await publicReleases(repo, tag)) ids.add(release.id)

    for (const id of ids) {
      // Run the same trusted immutable-execution gate used by provenance
      // withdrawal, immediately before EVERY target and ambiguous PATCH retry.
      // Keep authorization failures outside the catch that retries PATCHes.
      if (!sha40(executionSHA)) throw new Error("recovery execution SHA is invalid")
      execFileSync(process.execPath, [fileURLToPath(new URL("./release-publication-guard.mjs", import.meta.url)), "execution"], {
        stdio: "pipe", env: { ...process.env, RECOVERY_EXECUTION_SHA: executionSHA },
      })
      try {
        await api(`repos/${repo}/releases/${id}`, {
          method: "PATCH",
          body: { draft: true, prerelease: false, make_latest: "false" },
        })
      } catch (error) {
        console.warn(`Tagless-release withdrawal PATCH ${attempt} for release ${id} was ambiguous: ${error.message}`)
      }
    }

    if (await currentTag(repo, tag)) tagAppeared = true

    const after = await publicReleases(repo, tag)
    for (const release of after) ids.add(release.id)
    const byTag = await api(`repos/${repo}/releases/tags/${tag}`, { missing: true })
    const finalDirect = await directSnapshot(repo, ids)
    const directNotWithdrawn = finalDirect.some((release) => release && (release.draft !== true || release.prerelease !== false))
    const publicByTag = Boolean(byTag && byTag.draft === false)

    if (!directNotWithdrawn && !publicByTag && after.length === 0) {
      if (tagAppeared || await currentTag(repo, tag)) {
        throw new Error("release tag appeared while withdrawing tagless public release; publication was withdrawn but recovery cannot establish a new trust baseline")
      }
      console.warn(`Confirmed stable withdrawal of tagless public ${tag}; recovery may continue only from the exported explicit absent-tag state.`)
      return
    }

    await delay(Math.min(5000, attempt * 500))
  }
}

async function main() {
  const executionSHA = process.env.RECOVERY_EXECUTION_SHA
  const repo = repository()
  const version = readFileSync("VERSION", "utf8").trim()
  const tag = versionTag(version)
  const releases = await publicReleases(repo, tag)
  const tagState = await currentTag(repo, tag)

  if (!releases.length) {
    output("tag_absent", encodedTagSnapshot(tagState))
    return
  }

  if (tagState) {
    output("tag_absent", encodedTagSnapshot(tagState))
    return
  }

  await withdrawWhileTagAbsent(repo, tag, releases, executionSHA)
  output("tag_absent", "true")
}

main().catch((error) => {
  console.error(error.message)
  process.exitCode = 1
})
