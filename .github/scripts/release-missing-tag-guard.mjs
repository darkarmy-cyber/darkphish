import { appendFileSync, readFileSync } from "node:fs"
import { api, pages, repository, versionTag } from "../../scripts/release-lib.mjs"

const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const output = (name, value) => { if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, `${name}=${value}\n`) }
const sha40 = (value) => typeof value === "string" && /^[a-f0-9]{40}$/.test(value)
const actionsBot = (actor) => actor?.login === "github-actions[bot]" && actor?.type === "Bot" && actor?.id === 41898282

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

function notesForVersion(changelog, version) {
  const start = changelog.indexOf(`## ${version} - `)
  if (start < 0) throw new Error("duplicate-draft source changelog is missing the release section")
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
  if (file?.type !== "file" || file.encoding !== "base64" || typeof file.content !== "string") throw new Error(`duplicate-draft source ${path} is unavailable`)
  return Buffer.from(file.content.replace(/\n/g, ""), "base64").toString("utf8")
}

async function expectedMetadata(repo, source, version) {
  const notes = notesForVersion(await sourceText(repo, "CHANGELOG.md", source), version)
  return { name: releaseName(version), body: releaseBody(notes, source) }
}

function assertTrustedDraft(release, tag, source, expected) {
  if (!release || !Number.isSafeInteger(release.id) || release.id < 1 || release.tag_name !== tag ||
    release.target_commitish !== source || release.name !== expected.name || release.body !== expected.body ||
    release.draft !== true || release.prerelease !== false || release.published_at !== null || !actionsBot(release.author)) {
    throw new Error("duplicate pending release draft is not an exact trusted automation artifact")
  }
  return release
}

async function publicReleases(repo, tag) {
  return (await pages(`repos/${repo}/releases`)).filter((release) => release?.tag_name === tag && release.draft === false)
}

async function directSnapshot(repo, ids) {
  return Promise.all([...ids].map((id) => api(`repos/${repo}/releases/${id}`, { missing: true })))
}

async function reconcileDuplicateDrafts(repo, tag, version) {
  const tagged = (await pages(`repos/${repo}/releases`)).filter((release) => release?.tag_name === tag)
  const drafts = tagged.filter((release) => release.draft === true)
  if (drafts.length <= 1) return drafts[0] || null
  if (await currentTag(repo, tag)) throw new Error("release tag appeared before duplicate draft reconciliation")

  const sources = new Set(drafts.map((draft) => draft?.target_commitish))
  if (sources.size !== 1) throw new Error("pending release drafts disagree on immutable source")
  const source = [...sources][0]
  if (!sha40(source)) throw new Error("pending release draft source is malformed")
  const expected = await expectedMetadata(repo, source, version)
  for (const draft of drafts) assertTrustedDraft(draft, tag, source, expected)

  const ordered = [...drafts].sort((a, b) => a.id - b.id)
  const survivor = ordered[0]
  for (const stale of ordered.slice(1)) {
    if (await currentTag(repo, tag)) throw new Error("release tag appeared during duplicate draft reconciliation")
    const current = assertTrustedDraft(await api(`repos/${repo}/releases/${stale.id}`), tag, source, expected)
    if (current.id !== stale.id) throw new Error("duplicate release draft identity changed before deletion")
    await api(`repos/${repo}/releases/${stale.id}`, { method: "DELETE" })
    if (await api(`repos/${repo}/releases/${stale.id}`, { missing: true })) throw new Error("duplicate stale release draft still exists after deletion")
  }

  if (await currentTag(repo, tag)) throw new Error("release tag appeared after duplicate draft reconciliation")
  const remaining = (await pages(`repos/${repo}/releases`)).filter((release) => release?.tag_name === tag && release.draft === true)
  if (remaining.length !== 1 || remaining[0].id !== survivor.id) throw new Error("duplicate draft reconciliation did not leave exactly one trusted survivor")
  assertTrustedDraft(await api(`repos/${repo}/releases/${survivor.id}`), tag, source, expected)
  console.warn(`Removed ${ordered.length - 1} duplicate trusted ${tag} staging draft(s); immutable recovery will rebuild all release bytes.`)
  return survivor
}

async function withdrawWhileTagAbsent(repo, tag, releases) {
  let tagAppeared = false
  const ids = new Set(releases.filter((release) => Number.isSafeInteger(release?.id)).map((release) => release.id))
  for (let attempt = 1; ; attempt += 1) {
    for (const release of await publicReleases(repo, tag)) ids.add(release.id)

    for (const id of ids) {
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
  const repo = repository()
  const version = readFileSync("VERSION", "utf8").trim()
  const tag = versionTag(version)
  const releases = await publicReleases(repo, tag)
  const tagState = await currentTag(repo, tag)

  if (tagState) {
    output("tag_absent", encodedTagSnapshot(tagState))
    return
  }

  if (releases.length) await withdrawWhileTagAbsent(repo, tag, releases)
  await reconcileDuplicateDrafts(repo, tag, version)
  if (await currentTag(repo, tag)) throw new Error("release tag appeared after tagless recovery reconciliation")
  output("tag_absent", "true")
}

main().catch((error) => {
  console.error(error.message)
  process.exitCode = 1
})
