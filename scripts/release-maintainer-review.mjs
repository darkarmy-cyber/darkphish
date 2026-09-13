import { generatedPath } from "./release-lib.mjs"

export class ReleaseMaintainerReviewError extends Error {}

const trustedReleaseReviewer = (user) => user?.login === "oliverkko" && user?.id === 309485696 && user?.type === "User"
const trustedActionsActor = (user) => user?.login === "github-actions[bot]" && user?.id === 41898282 && user?.type === "Bot"
const markerPattern = /<!-- darkphish-release-maintainer-review:v1 (.*?) -->/g
const expectedPrefix = "Darkphish generated-release review completed for this exact head."

function requireReview(condition, message) {
  if (!condition) throw new ReleaseMaintainerReviewError(message)
}

function timestamp(value) {
  requireReview(typeof value === "string" && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value), "Invalid release review timestamp")
  const result = Date.parse(value)
  requireReview(Number.isFinite(result), "Invalid release review timestamp")
  return result
}

async function readPages(get, path) {
  const all = [], seen = new Set()
  for (let page = 1; page <= 20; page++) {
    const separator = path.includes("?") ? "&" : "?"
    const rows = await get(`${path}${separator}per_page=100&page=${page}`)
    requireReview(Array.isArray(rows) && rows.length <= 100, "Invalid release review evidence page")
    for (const row of rows) {
      requireReview(Number.isSafeInteger(row?.id) && row.id > 0 && !seen.has(row.id), "Duplicate or invalid release review evidence identity")
      seen.add(row.id)
      all.push(row)
    }
    if (rows.length < 100) return all
  }
  throw new ReleaseMaintainerReviewError("Release review pagination limit reached; manual investigation required")
}

function parseAttestation(repo, pr, review) {
  requireReview(typeof review?.body === "string" && review.body.startsWith(expectedPrefix + "\n"), "Missing explicit generated-release review statement")
  const markers = [...review.body.matchAll(markerPattern)]
  requireReview(markers.length === 1, "Missing or ambiguous generated-release review marker")
  let attestation
  try { attestation = JSON.parse(markers[0][1]) } catch { throw new ReleaseMaintainerReviewError("Malformed generated-release review marker") }
  requireReview(attestation && typeof attestation === "object" && !Array.isArray(attestation), "Malformed generated-release review attestation")
  requireReview(Object.keys(attestation).sort().join("\n") === ["baseSha", "codeReview", "decision", "headSha", "pullRequestNumber", "repository", "scope", "securityReview"].sort().join("\n"), "Unexpected generated-release review attestation fields")
  requireReview(attestation.repository === repo && attestation.pullRequestNumber === pr.number &&
    attestation.headSha === pr.head.sha && attestation.baseSha === pr.base.sha &&
    attestation.scope === "generated-release-only" && attestation.codeReview === "completed" &&
    attestation.securityReview === "completed" && attestation.decision === "approved",
  "Generated-release review does not certify this exact PR head and base")
  return attestation
}

export function releaseReviewBody(repo, pr) {
  requireReview(/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repo) && Number.isSafeInteger(pr?.number) && pr.number > 0 &&
    /^[a-f0-9]{40}$/.test(pr?.head?.sha || "") && /^[a-f0-9]{40}$/.test(pr?.base?.sha || ""), "Invalid release review body target")
  const marker = JSON.stringify({
    repository: repo,
    pullRequestNumber: pr.number,
    headSha: pr.head.sha,
    baseSha: pr.base.sha,
    scope: "generated-release-only",
    codeReview: "completed",
    securityReview: "completed",
    decision: "approved",
  })
  return `${expectedPrefix}\n\nVerified generated-file scope, version/changelog integrity, exact-head CI and CodeQL provenance, resolved review state, and absence of unrelated executable or workflow changes.\n\n<!-- darkphish-release-maintainer-review:v1 ${marker} -->`
}

export async function verifyReleaseMaintainerReview(repo, pr, { get }) {
  requireReview(/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repo) && Number.isSafeInteger(pr?.number) && pr.number > 0,
    "Invalid generated-release review target")
  requireReview(trustedActionsActor(pr.user) && pr.draft === false && pr.base?.ref === "main" && /^[a-f0-9]{40}$/.test(pr.base?.sha || "") &&
    pr.head?.repo?.full_name === repo && /^release\/v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(pr.head?.ref || "") &&
    /^[a-f0-9]{40}$/.test(pr.head?.sha || "") && pr.title === `release: Darkphish ${pr.head.ref.slice("release/v".length)}`,
  "PR is not a trusted generated release pull request")

  const files = await readPages(get, `repos/${repo}/pulls/${pr.number}/files`)
  requireReview(files.length > 0 && files.every((file) => typeof file?.filename === "string" && generatedPath(file.filename)),
    "Generated release review target includes application or unexpected files")

  const reviews = await readPages(get, `repos/${repo}/pulls/${pr.number}/reviews`)
  const opinions = new Map()
  for (const review of reviews.sort((a, b) => a.id - b.id)) {
    requireReview(typeof review.user?.login === "string" && ["APPROVED", "CHANGES_REQUESTED", "COMMENTED", "DISMISSED", "PENDING"].includes(review.state),
      "Malformed GitHub release review")
    requireReview(review.state !== "PENDING", "A GitHub release review is still pending")
    if (["APPROVED", "CHANGES_REQUESTED"].includes(review.state)) opinions.set(review.user.login, review.state)
  }
  requireReview(![...opinions.values()].includes("CHANGES_REQUESTED"), "A reviewer still requests changes on the release PR")

  const trusted = reviews.filter((review) => trustedReleaseReviewer(review.user))
  requireReview(trusted.length > 0, "Missing trusted maintainer generated-release review")
  const latest = trusted.reduce((current, review) => !current || review.id > current.id ? review : current, null)
  requireReview(latest.state === "APPROVED" && latest.commit_id === pr.head.sha, "Trusted maintainer review is not an approval of the exact release head")
  parseAttestation(repo, pr, latest)
  const submitted = timestamp(latest.submitted_at)
  if (pr.merged_at) requireReview(submitted <= timestamp(pr.merged_at), "Generated-release review was submitted after merge")

  const finalPR = await get(`repos/${repo}/pulls/${pr.number}`)
  requireReview(finalPR.number === pr.number && finalPR.head?.repo?.full_name === repo && finalPR.head?.ref === pr.head.ref &&
    finalPR.head?.sha === pr.head.sha && finalPR.base?.ref === "main" && finalPR.base?.sha === pr.base.sha && finalPR.title === pr.title &&
    finalPR.state === pr.state && finalPR.merged_at === pr.merged_at, "Release PR changed during maintainer review verification")
  return { head: pr.head.sha, base: pr.base.sha, reviewID: latest.id, reviewer: latest.user.login, submittedAt: latest.submitted_at }
}
