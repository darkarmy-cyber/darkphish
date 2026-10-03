import { api, assertCurrentVersionPublished, pages, versionTag } from "./release-lib.mjs"

export async function assertReleaseAdvancePublished(repo, current, target, { verify = assertCurrentVersionPublished, request = api } = {}) {
  versionTag(current)
  versionTag(target)
  if (target === current) return
  if (repo === "darkarmy-cyber/darkphish" && current === "0.25.1" && target === "0.25.2") {
    const tag = await request(`repos/${repo}/git/ref/tags/v0.25.1`, { missing: true })
    const release = await request(`repos/${repo}/releases/tags/v0.25.1`, { missing: true })
    const pr = await request(`repos/${repo}/pulls/196`, { missing: true })
    const audited = pr?.number === 196 && pr.state === "closed" && pr.draft === false &&
      pr.title === "release: Darkphish 0.25.1" && pr.merged_at === "2026-10-02T21:03:30Z" &&
      pr.merge_commit_sha === "98d96c7cc7eebf6ab09bbeffdf109778dd4f3632" &&
      pr.base?.ref === "main" && pr.base?.sha === "ffba3cab315c4580695d69c72ed0539e8b6a1d46" &&
      pr.head?.ref === "release/v0.25.1" && pr.head?.sha === "d64cd123501fd2d7b5976996a2ddef072d96b041" &&
      pr.head?.repo?.full_name === repo
    if (!audited || tag || release) throw new Error("audited v0.25.1 bridge state changed; refusing release advance")
    return
  }
  if (repo === "darkarmy-cyber/darkphish" && current === "0.23.1" && target === "0.24.0") {
    const tag = await request(`repos/${repo}/git/ref/tags/v0.23.1`, { missing: true })
    const release = await request(`repos/${repo}/releases/tags/v0.23.1`, { missing: true })
    const pr = await request(`repos/${repo}/pulls/181`, { missing: true })
    const audited = pr?.number === 181 && pr.state === "closed" && pr.draft === false &&
      pr.title === "release: Darkphish 0.23.1" && pr.merged_at === "2026-10-02T12:24:22Z" &&
      pr.merge_commit_sha === "86cb7173c553d68bc3afac204948aa923a32bc83" &&
      pr.base?.ref === "main" && pr.base?.sha === "443d247b1636ea8e46c7339378174d3c88a0abf5" &&
      pr.head?.ref === "release/v0.23.1" && pr.head?.sha === "5f8b6de5a6ed4076b987747c32d615b5b9bd180a" &&
      pr.head?.repo?.full_name === repo
    if (!audited || tag || release) throw new Error("audited v0.23.1 bridge state changed; refusing release advance")
    return
  }
  await verify(repo, { version: current })
}

// A merged release without a tag or staging draft is unfinished work, not a
// successful recovery no-op. This diagnostic does not authorize publication or
// substitute a PR for the original build, review and attestation evidence.
export async function assertNoUnstagedRelease(repo, version, { request = api } = {}) {
  const tag = versionTag(version)
  const prs = await pages(`repos/${repo}/pulls?state=closed&base=main&head=${repo.split("/")[0]}:release/${tag}`, undefined, request)
  const merged = prs.filter(pr => pr.state === "closed" && pr.merged_at &&
    pr.base?.ref === "main" && pr.head?.repo?.full_name === repo &&
    pr.head?.ref === `release/${tag}` && pr.title === `release: Darkphish ${version}`)
  if (merged.length) {
    throw new Error(`${tag} has merged release PRs (${merged.map(pr => `#${pr.number}`).join(", ")}) but no tag or staging draft; inspect Native release review/build provenance before recovery`)
  }
}
