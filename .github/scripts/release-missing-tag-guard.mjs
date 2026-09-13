import { readFileSync } from "node:fs"
import { api, pages, repository, versionTag } from "../../scripts/release-lib.mjs"

const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

async function currentTag(repo, tag) {
  return api(`repos/${repo}/git/ref/tags/${tag}`, { missing: true })
}

async function publicReleases(repo, tag) {
  return (await pages(`repos/${repo}/releases`)).filter((release) => release?.tag_name === tag && release.draft === false)
}

async function withdrawWhileTagAbsent(repo, tag, releases) {
  for (let attempt = 1; ; attempt += 1) {
    if (await currentTag(repo, tag)) throw new Error("release tag appeared while withdrawing tagless public release; refusing to establish a new trust baseline")

    for (const release of releases) {
      try {
        await api(`repos/${repo}/releases/${release.id}`, {
          method: "PATCH",
          body: { draft: true, prerelease: false, make_latest: "false" },
        })
      } catch (error) {
        console.warn(`Tagless-release withdrawal PATCH ${attempt} for release ${release.id} was ambiguous: ${error.message}`)
      }
    }

    releases = await publicReleases(repo, tag)
    const tagState = await currentTag(repo, tag)
    if (!releases.length && !tagState) {
      console.warn(`Confirmed withdrawal of tagless public ${tag}; recovery may continue from an explicit absent-tag state.`)
      return
    }
    if (tagState) throw new Error("release tag appeared during tagless-release withdrawal; fail closed")
    await delay(Math.min(5000, attempt * 500))
  }
}

async function main() {
  const repo = repository()
  const version = readFileSync("VERSION", "utf8").trim()
  const tag = versionTag(version)
  const releases = await publicReleases(repo, tag)
  if (!releases.length) return
  if (await currentTag(repo, tag)) return
  await withdrawWhileTagAbsent(repo, tag, releases)
}

main().catch((error) => {
  console.error(error.message)
  process.exitCode = 1
})
