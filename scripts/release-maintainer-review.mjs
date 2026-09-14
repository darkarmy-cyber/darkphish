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

async function readPages(get, path, identity) {
  const all = [], seen = new Set()
  for (let page = 1; page <= 20; page++) {
    const separator = path.includes("?") ? "&" : "?"
    const rows = await get(`${path}${separator}per_page=100&page=${page}`)
    requireReview(Array.isArray(rows) && rows.length <= 100, "Invalid release review evidence page")
    for (const row of rows) {
      const key = identity(row)
      requireReview(typeof key === "string" && key.length > 0 && !seen.has(key), "Duplicate or invalid release review evidence identity")
      seen.add(key)
      all.push(row)
    }
    if (rows.length < 100) return all
  }
  throw new ReleaseMaintainerReviewError("Release review pagination limit reached; manual investigation required")
}

const reviewIdentity = (review) => Number.isSafeInteger(review?.id) && review.id > 0 ? `review:${review.id}` : null
const fileIdentity = (file) => typeof file?.filename === "string" && file.filename.length > 0 && /^[a-f0-9]{40}$/.test(file?.sha || "")
  ? `file:${file.filename}:${file.sha}` : null

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

async function verifyReviewGraphQL(get, review) {
  requireReview(typeof review?.node_id === "string" && review.node_id.length > 0, "Trusted maintainer review is missing immutable GraphQL identity")
  const result = await get("graphql", { method: "POST", body: {
    query: `query($id: ID!) {
      node(id: $id) {
        ... on PullRequestReview {
          databaseId
          body
          submittedAt
          lastEditedAt
          author { __typename ... on User { databaseId login } }
          editor { __typename ... on User { databaseId login } }
        }
      }
    }`,
    variables: { id: review.node_id },
  } })
  requireReview(!result?.errors && result?.data?.node, "Unable to verify trusted maintainer review provenance")
  const node = result.data.node
  requireReview(node.databaseId === review.id && node.body === review.body && node.submittedAt === review.submitted_at,
    "Trusted maintainer review provenance does not match REST evidence")
  requireReview(node.author?.__typename === "User" && node.author?.databaseId === 309485696 && node.author?.login === "oliverkko",
    "Trusted maintainer review GraphQL author is not the pinned repository owner")
  requireReview(node.lastEditedAt === null && node.editor === null,
    "Trusted maintainer release attestation was edited after submission")
}

async function verifyResolvedThreads(get, repo, pr) {
  let after = null
  for (let page = 1; page <= 20; page++) {
    const result = await get("graphql", { method: "POST", body: {
      query: `query($owner: String!, $name: String!, $number: Int!, $after: String) {
        repository(owner: $owner, name: $name) {
          pullRequest(number: $number) {
            headRefOid
            reviewThreads(first: 100, after: $after) {
              nodes { id isResolved }
              pageInfo { hasNextPage endCursor }
            }
          }
        }
      }`,
      variables: { owner: repo.split("/")[0], name: repo.split("/")[1], number: pr.number, after },
    } })
    requireReview(!result?.errors && result?.data?.repository?.pullRequest, "Unable to verify release review threads")
    const pull = result.data.repository.pullRequest
    requireReview(pull.headRefOid === pr.head.sha, "Release PR head changed while verifying review threads")
    const threads = pull.reviewThreads
    requireReview(Array.isArray(threads?.nodes) && threads?.pageInfo && typeof threads.pageInfo.hasNextPage === "boolean",
      "Invalid release review thread page")
    const ids = new Set()
    for (const thread of threads.nodes) {
      requireReview(typeof thread?.id === "string" && thread.id.length > 0 && !ids.has(thread.id) && typeof thread.isResolved === "boolean",
        "Invalid release review thread identity")
      ids.add(thread.id)
      requireReview(thread.isResolved, "An unresolved review thread still blocks the release PR")
    }
    if (!threads.pageInfo.hasNextPage) return
    requireReview(typeof threads.pageInfo.endCursor === "string" && threads.pageInfo.endCursor.length > 0,
      "Invalid release review thread pagination cursor")
    after = threads.pageInfo.endCursor
  }
  throw new ReleaseMaintainerReviewError("Release review thread pagination limit reached; manual investigation required")
}

async function verifyLegacyProtectedAutoMerge(get, repo, pr) {
  requireReview(pr.merged_at && /^[a-f0-9]{40}$/.test(pr.merge_commit_sha || ""), "Historical release PR lacks immutable merge provenance")
  const file = await get(`repos/${repo}/contents/scripts/release-lib.mjs?ref=${pr.merge_commit_sha}`)
  requireReview(file?.type === "file" && file.encoding === "base64" && typeof file.content === "string", "Historical release merge policy source is unavailable")
  const source = Buffer.from(file.content.replace(/\n/g, ""), "base64").toString("utf8")
  requireReview(!source.includes("verifyPullRequestReviews") && !source.includes("release-maintainer-review") && !source.includes("review-gate"),
    "Historical release source already required explicit review provenance")
  requireReview(source.includes("export function protectedMergeArguments(repo, pr)") &&
    source.includes('["pr", "merge", String(pr.number), "--repo", repo, "--auto", "--squash", "--match-head-commit", pr.head.sha]') &&
    source.includes('if (!metadata.allow_auto_merge || !base.protected) throw new Error("native auto-merge requires enabled repository auto-merge and a protected base branch")') &&
    source.includes('if (pr.draft || pr.head.repo?.full_name !== repo) throw new Error("only internal ready pull requests are eligible")'),
  "Historical release source does not prove the protected exact-head auto-merge policy")
  await verifyResolvedThreads(get, repo, pr)
  const finalPR = await get(`repos/${repo}/pulls/${pr.number}`)
  requireReview(finalPR.number === pr.number && finalPR.head?.repo?.full_name === repo && finalPR.head?.ref === pr.head.ref &&
    finalPR.head?.sha === pr.head.sha && finalPR.base?.ref === "main" && finalPR.base?.sha === pr.base.sha && finalPR.title === pr.title &&
    finalPR.state === pr.state && finalPR.merged_at === pr.merged_at && finalPR.merge_commit_sha === pr.merge_commit_sha,
  "Historical release PR changed during protected auto-merge verification")
  return { head: pr.head.sha, base: pr.base.sha, legacyProtectedAutoMerge: true, mergeCommit: pr.merge_commit_sha }
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

  const files = await readPages(get, `repos/${repo}/pulls/${pr.number}/files`, fileIdentity)
  requireReview(files.length > 0 && files.every((file) => typeof file?.filename === "string" && generatedPath(file.filename)),
    "Generated release review target includes application or unexpected files")

  const reviews = await readPages(get, `repos/${repo}/pulls/${pr.number}/reviews`, reviewIdentity)
  const opinions = new Map()
  for (const review of reviews.sort((a, b) => a.id - b.id)) {
    requireReview(typeof review.user?.login === "string" && ["APPROVED", "CHANGES_REQUESTED", "COMMENTED", "DISMISSED", "PENDING"].includes(review.state),
      "Malformed GitHub release review")
    requireReview(review.state !== "PENDING", "A GitHub release review is still pending")
    if (["APPROVED", "CHANGES_REQUESTED"].includes(review.state)) opinions.set(review.user.login, review.state)
  }
  requireReview(![...opinions.values()].includes("CHANGES_REQUESTED"), "A reviewer still requests changes on the release PR")

  const trusted = reviews.filter((review) => trustedReleaseReviewer(review.user))
  if (trusted.length === 0) {
    requireReview(reviews.length === 0, "Missing trusted maintainer generated-release review")
    return verifyLegacyProtectedAutoMerge(get, repo, pr)
  }
  const latest = trusted.reduce((current, review) => !current || review.id > current.id ? review : current, null)
  requireReview(latest.state === "APPROVED" && latest.commit_id === pr.head.sha, "Trusted maintainer review is not an approval of the exact release head")
  parseAttestation(repo, pr, latest)
  const submitted = timestamp(latest.submitted_at)
  if (pr.merged_at) requireReview(submitted <= timestamp(pr.merged_at), "Generated-release review was submitted after merge")

  await verifyReviewGraphQL(get, latest)
  await verifyResolvedThreads(get, repo, pr)

  const finalPR = await get(`repos/${repo}/pulls/${pr.number}`)
  requireReview(finalPR.number === pr.number && finalPR.head?.repo?.full_name === repo && finalPR.head?.ref === pr.head.ref &&
    finalPR.head?.sha === pr.head.sha && finalPR.base?.ref === "main" && finalPR.base?.sha === pr.base.sha && finalPR.title === pr.title &&
    finalPR.state === pr.state && finalPR.merged_at === pr.merged_at, "Release PR changed during maintainer review verification")
  return { head: pr.head.sha, base: pr.base.sha, reviewID: latest.id, reviewer: latest.user.login, submittedAt: latest.submitted_at }
}
