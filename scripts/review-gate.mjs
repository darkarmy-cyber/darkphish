// The connector's summary format is a preview integration, not a stable API.
// Recognize only observed positive evidence; format drift must stop automation.
export class ReviewGateError extends Error {}
const requireReview = (condition, message) => { if (!condition) throw new ReviewGateError(message) }
const summaryMarker = "<!-- codex-pull-request-review-summary -->"
const cleanMessages = {
  code: "Codex Review: Didn't find any major issues.",
  security: "Security review completed. No security issues were found in this pull request.",
}

export function trustedReviewComment(comment) {
  return comment?.user?.login === "chatgpt-codex-connector[bot]" && comment.user.id === 199175422 &&
    comment.user.type === "Bot" && comment.performed_via_github_app?.id === 1144995 &&
    comment.performed_via_github_app.slug === "chatgpt-codex-connector"
}
function timestamp(value) {
  requireReview(typeof value === "string" && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value), "Invalid review timestamp")
  const result = Date.parse(value)
  requireReview(Number.isFinite(result), "Invalid review timestamp")
  return result
}
function completedRow(body, kind, sha, latest) {
  const rows = body.split("\n").filter(line => line.includes(`**${kind} Review**`))
  requireReview(rows.length === 1, `Missing or ambiguous ${kind} review status`)
  const cells = rows[0].split("|").map(cell => cell.trim())
  requireReview(cells.length === 6 && cells[0] === "" && cells[5] === "", "Invalid review summary row")
  const status = /^✅ \*\*Completed\*\* <relative-time datetime="([^"]+)">\1<\/relative-time>$/.exec(cells[2])
  const commit = /^`([a-f0-9]{7,40})`$/.exec(cells[3])
  requireReview(status && commit && sha.startsWith(commit[1]), `${kind} review is incomplete or stale`)
  if (kind === "Security") requireReview(cells[4] === "Manual request", "Explicit security review is required")
  const completed = timestamp(status[1])
  requireReview(completed <= latest, "Review completed after the permitted merge boundary")
  return completed
}

export function reviewEvidence(repo, pr, comments, now = Date.now()) {
  requireReview(/^[a-f0-9]{40}$/.test(pr.head?.sha || ""), "Invalid review head")
  requireReview(Array.isArray(comments), "Invalid review comment collection")
  const trusted = comments.filter(trustedReviewComment)
  const summaries = trusted.filter(comment => typeof comment.body === "string" && comment.body.startsWith(summaryMarker + "\n"))
  requireReview(summaries.length === 1, "Missing or ambiguous trusted review summary")
  const summary = summaries[0]
  requireReview(!/^#{1,6}\s+[^\n]*\bfindings?\b/im.test(summary.body), "Latest review summary reports findings")
  const markers = [...summary.body.matchAll(/<!-- codex-security-review:v1 (.*?) -->/g)]
  requireReview(markers.length === 1, "Missing or ambiguous explicit security review marker")
  let security
  try { security = JSON.parse(markers[0][1]) } catch { throw new ReviewGateError("Malformed security review marker") }
  requireReview(security?.repository === repo && security.pullRequestNumber === pr.number &&
    security.headSha === pr.head.sha && security.status === "completed", "Security review does not certify this PR head")
  const latest = pr.merged_at ? timestamp(pr.merged_at) : now
  const completed = {
    code: completedRow(summary.body, "Code", pr.head.sha, latest),
    security: completedRow(summary.body, "Security", pr.head.sha, latest),
  }
  requireReview(timestamp(summary.created_at) <= Math.min(...Object.values(completed)) &&
    timestamp(summary.updated_at) >= Math.max(...Object.values(completed)), "Inconsistent review summary timestamps")
  const clean = {}
  for (const [kind, message] of Object.entries(cleanMessages)) {
    const candidates = trusted.filter(comment => typeof comment.body === "string" && comment.body.startsWith(message))
      .sort((a, b) => b.id - a.id)
    // Never fall back to a historical clean result when the newest one is for
    // another head or is malformed. Formal reviews with findings are checked
    // separately below; a clean issue comment is not the entire result history.
    const comment = candidates[0]
    if (comment) {
      const matches = [...comment.body.matchAll(/^\*\*Reviewed commit:\*\* `([a-f0-9]{10,40})`$/gm)]
      requireReview(matches.length === 1 && pr.head.sha.startsWith(matches[0][1]), `Latest clean ${kind} result is malformed or stale`)
      requireReview(timestamp(comment.created_at) <= completed[kind] && timestamp(comment.updated_at) <= latest, "Clean review evidence is newer than its completion")
      clean[kind] = { id: comment.id, nodeID: comment.node_id, body: comment.body, createdAt: comment.created_at, updatedAt: comment.updated_at, prefix: matches[0][1] }
    }
    requireReview(clean[kind], `Missing clean ${kind} review for this head`)
  }
  const firstClean = Math.min(...Object.values(clean).map(item => timestamp(item.createdAt)))
  for (const comment of trusted) {
    if (comment.id === summary.id || Object.values(cleanMessages).some(message => comment.body?.startsWith(message))) continue
    requireReview(timestamp(comment.created_at) < firstClean, "A later unrecognized connector result supersedes clean review evidence")
  }
  const records = [{ id: summary.id, nodeID: summary.node_id, body: summary.body, updatedAt: summary.updated_at }, ...Object.values(clean)]
  requireReview(records.every(record => typeof record.nodeID === "string" && record.nodeID.length > 0 && record.nodeID.length < 128), "Missing review comment node identity")
  return { head: pr.head.sha, summaryID: summary.id, completed, clean, records }
}

async function readPages(get, path) {
  const all = [], seen = new Set()
  for (let page = 1; page <= 20; page++) {
    const rows = await get(`${path}?per_page=100&page=${page}`)
    requireReview(Array.isArray(rows) && rows.length <= 100, "Invalid review evidence page")
    for (const row of rows) {
      requireReview(Number.isSafeInteger(row?.id) && row.id > 0 && !seen.has(row.id), "Duplicate or invalid review evidence identity")
      seen.add(row.id)
      all.push(row)
    }
    if (rows.length < 100) return all
  }
  throw new ReviewGateError("Review pagination limit reached; manual investigation required")
}

export async function verifyPullRequestReviews(repo, pr, { get, query, now = Date.now() }) {
  requireReview(/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repo) && !repo.split("/").some(part => part === "." || part === "..") &&
    Number.isSafeInteger(pr.number) && pr.number > 0 && pr.head?.repo?.full_name === repo && pr.base?.ref === "main", "Invalid review target")
  const evidence = reviewEvidence(repo, pr, await readPages(get, `repos/${repo}/issues/${pr.number}/comments`), now)
  for (const prefix of new Set(Object.values(evidence.clean).map(item => item.prefix))) {
    if (prefix.length === 40) continue
    // Do not let a branch/tag named like an abbreviated SHA shadow a commit.
    for (const type of ["heads", "tags"]) requireReview(await get(`repos/${repo}/git/ref/${type}/${prefix}`, { missing: true }) === null, "Review commit abbreviation is shadowed by a ref")
    const commit = await get(`repos/${repo}/commits/${prefix}`)
    requireReview(commit?.sha === pr.head.sha, "Review commit abbreviation does not resolve to the full expected SHA")
  }
  const reviews = await readPages(get, `repos/${repo}/pulls/${pr.number}/reviews`)
  const opinions = new Map()
  for (const review of reviews.sort((a, b) => a.id - b.id)) {
    requireReview(typeof review.user?.login === "string" && ["APPROVED", "CHANGES_REQUESTED", "COMMENTED", "DISMISSED", "PENDING"].includes(review.state), "Malformed GitHub review")
    requireReview(review.state !== "PENDING", "A GitHub review is still pending")
    // Connector findings are formal PR reviews, not clean issue comments.
    // Immutable submission identity/time remains a barrier even if the body is
    // edited, the review dismissed or all its threads marked resolved. Since
    // the preview API exposes no run ID, conservatively require BOTH fresh clean
    // results after any connector formal review (also a late older-head result).
    if (review.user.id === 199175422 || review.user.login === "chatgpt-codex-connector[bot]") {
      requireReview(review.user.id === 199175422 && review.user.type === "Bot" &&
        review.user.login === "chatgpt-codex-connector[bot]" && /^[a-f0-9]{40}$/.test(review.commit_id || ""), "Malformed connector review identity")
      const submitted = timestamp(review.submitted_at)
      requireReview(Object.values(evidence.clean).every(item => timestamp(item.createdAt) > submitted),
        "A later connector review supersedes clean results; both fresh clean reviews are required")
    }
    if (["APPROVED", "CHANGES_REQUESTED"].includes(review.state)) opinions.set(review.user.login, review.state)
  }
  requireReview(![...opinions.values()].includes("CHANGES_REQUESTED"), "A reviewer still requests changes")
  const [owner, name] = repo.split("/")
  let after = null
  const cursors = new Set()
  for (let page = 0; page < 20; page++) {
    const result = await query({ query: `query($owner:String!,$name:String!,$number:Int!,$after:String) {
      repository(owner:$owner,name:$name) { pullRequest(number:$number) {
        headRefOid reviewThreads(first:100,after:$after) { nodes { isResolved } pageInfo { hasNextPage endCursor } }
      } }
    }`, variables: { owner, name, number: pr.number, after } })
    requireReview(!result?.errors, "GitHub review-thread query failed")
    const current = result?.data?.repository?.pullRequest, threads = current?.reviewThreads
    requireReview(current?.headRefOid === pr.head.sha && Array.isArray(threads?.nodes) && threads.nodes.length <= 100 &&
      typeof threads.pageInfo?.hasNextPage === "boolean", "Malformed or changing review-thread snapshot")
    requireReview(threads.nodes.every(thread => thread?.isResolved === true), "Unresolved review threads remain")
    if (!threads.pageInfo.hasNextPage) break
    after = threads.pageInfo.endCursor
    requireReview(typeof after === "string" && after && !cursors.has(after) && page < 19, "Invalid or excessive review-thread pagination")
    cursors.add(after)
  }
  // A repository maintainer can edit somebody else's issue comment. Creation
  // identity alone therefore does not authenticate its current content.
  const content = await query({ query: `query($ids:[ID!]!) { nodes(ids:$ids) { ... on IssueComment {
    databaseId body updatedAt lastEditedAt author { __typename ... on Bot { databaseId } }
    editor { __typename ... on Bot { databaseId } }
  } } }`, variables: { ids: evidence.records.map(record => record.nodeID) } })
  const isConnector = actor => actor?.__typename === "Bot" && actor.databaseId === 199175422
  requireReview(!content?.errors && Array.isArray(content?.data?.nodes) && content.data.nodes.length === 3, "Missing review content provenance")
  for (const record of evidence.records) {
    const matches = content.data.nodes.filter(node => node?.databaseId === record.id)
    requireReview(matches.length === 1, "Ambiguous review content identity")
    const node = matches[0]
    requireReview(isConnector(node.author) && (node.lastEditedAt === null ? node.editor === null : isConnector(node.editor)) &&
      node.body === record.body && node.updatedAt === record.updatedAt, "Review changed or was edited outside the trusted connector")
  }
  const finalPR = await get(`repos/${repo}/pulls/${pr.number}`)
  requireReview(finalPR.number === pr.number && finalPR.head?.repo?.full_name === repo && finalPR.base?.ref === "main" &&
    finalPR.head?.sha === pr.head.sha && finalPR.base?.sha === pr.base.sha && finalPR.state === pr.state &&
    finalPR.merged_at === pr.merged_at, "PR changed during review verification")
  return evidence
}
