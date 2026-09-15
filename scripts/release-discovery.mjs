import { api, pages } from "./release-lib.mjs"

function assertIdentity(release, tag, id = release?.id) {
  if (!Number.isSafeInteger(id) || id <= 0 || release?.id !== id || release.tag_name !== tag) {
    throw new Error("release discovery identity changed")
  }
}

// The by-tag endpoint discovers published releases, not all staging drafts.
// Use the authenticated, fully paginated list only when publication is absent.
// Never adopt an arbitrary first match or a detached untagged draft.
export async function discoverRelease(repo, tag, { request = api } = {}) {
  const byTag = await request(`repos/${repo}/releases/tags/${tag}`, { missing: true })
  if (byTag) {
    assertIdentity(byTag, tag)
    if (byTag.draft === false) return byTag
  }
  const matches = (await pages(`repos/${repo}/releases`, undefined, request)).filter(item => item?.tag_name === tag)
  if (matches.length > 1) throw new Error("multiple release candidates require manual investigation")
  const candidate = matches[0]
  if (!candidate) {
    if (byTag) throw new Error("release disappeared during discovery")
    return null
  }
  assertIdentity(candidate, tag)
  if (byTag && byTag.id !== candidate.id) throw new Error("release endpoints disagree on identity")
  if (candidate.draft !== true) throw new Error("release publication changed during discovery")
  const release = await request(`repos/${repo}/releases/${candidate.id}`)
  assertIdentity(release, tag, candidate.id)
  for (const key of ["draft", "prerelease", "target_commitish", "name", "body"]) {
    if (release[key] !== candidate[key]) throw new Error("release metadata changed during discovery")
  }
  for (const key of ["login", "type", "id"]) {
    if (release.author?.[key] !== candidate.author?.[key]) throw new Error("release author changed during discovery")
  }
  return release
}

export function assertStagingMetadata(release, { id = release?.id, tag, name, body }) {
  assertIdentity(release, tag, id)
  if (release.name !== name || release.body !== body) throw new Error("release metadata differs from canonical source")
}
