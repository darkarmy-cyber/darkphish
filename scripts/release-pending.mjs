import { api, assertCurrentVersionPublished, pages, versionTag } from "./release-lib.mjs"

export async function assertReleaseAdvancePublished(repo, current, target, { verify = assertCurrentVersionPublished } = {}) {
  versionTag(current)
  versionTag(target)
  if (target !== current) await verify(repo, { version: current })
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
