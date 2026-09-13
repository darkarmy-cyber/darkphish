import { appendFileSync, readFileSync } from "node:fs"
import { api, pages, repository, versionTag } from "../../scripts/release-lib.mjs"

const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const output = (name, value) => { if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, `${name}=${value}\n`) }

async function currentTag(repo, tag) {
  return api(`repos/${repo}/git/ref/tags/${tag}`, { missing: true })
}

async function publicReleases(repo, tag) {
  return (await pages(`repos/${repo}/releases`)).filter((release) => release?.tag_name === tag && release.draft === false)
}

async function withdrawWhileTagAbsent(repo, tag, releases) {
  let tagAppeared = false
  for (let attempt = 1; ; attempt += 1) {
    const candidates = new Map(releases.filter((release) => Number.isSafeInteger(release?.id)).map((release) => [release.id, release]))
    for (const release of await publicReleases(repo, tag)) candidates.set(release.id, release)

    for (const release of candidates.values()) {
      try {
        await api(`repos/${repo}/releases/${release.id}`, {
          method: "PATCH",
          body: { draft: true, prerelease: false, make_latest: "false" },
        })
      } catch (error) {
        console.warn(`Tagless-release withdrawal PATCH ${attempt} for release ${release.id} was ambiguous: ${error.message}`)
      }
    }

    const tagState = await currentTag(repo, tag)
    if (tagState) tagAppeared = true

    const ids = [...candidates.keys()]
    const direct = await Promise.all(ids.map((id) => api(`repos/${repo}/releases/${id}`, { missing: true })))
    const byTag = await api(`repos/${repo}/releases/tags/${tag}`, { missing: true })
    const after = await publicReleases(repo, tag)
    const directPublic = direct.some((release) => release && release.draft === false && release?.tag_name === tag)
    const publicByTag = Boolean(byTag && byTag.draft === false)

    if (!directPublic && !publicByTag && after.length === 0) {
      if (tagAppeared || await currentTag(repo, tag)) {
        throw new Error("release tag appeared while withdrawing tagless public release; publication was withdrawn but recovery cannot establish a new trust baseline")
      }
      console.warn(`Confirmed withdrawal of tagless public ${tag}; recovery may continue only from the exported explicit absent-tag state.`)
      return
    }

    releases = after
    await delay(Math.min(5000, attempt * 500))
  }
}

async function main() {
  const repo = repository()
  const version = readFileSync("VERSION", "utf8").trim()
  const tag = versionTag(version)
  const releases = await publicReleases(repo, tag)
  const tagState = await currentTag(repo, tag)

  if (!releases.length) {
    output("tag_absent", tagState ? "false" : "true")
    return
  }

  if (tagState) {
    output("tag_absent", "false")
    return
  }

  await withdrawWhileTagAbsent(repo, tag, releases)
  output("tag_absent", "true")
}

main().catch((error) => {
  console.error(error.message)
  process.exitCode = 1
})
