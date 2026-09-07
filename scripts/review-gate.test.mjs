import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"
import { eligibleEngineeringPR } from "./automerge.mjs"
import { mergeReviewedPullRequest, requiredChecks } from "./release-lib.mjs"
import { ReviewGateError, reviewEvidence, verifyPullRequestReviews } from "./review-gate.mjs"

const repo = "owner/repository", sha = "a".repeat(40), base = "b".repeat(40)
const at = seconds => `2026-09-07T00:00:${String(seconds).padStart(2, "0")}Z`
function fixture() {
  const pr = { number: 42, state: "open", draft: false, merged_at: null, auto_merge: null,
    head: { sha, repo: { full_name: repo } }, base: { ref: "main", sha: base },
    author_association: "OWNER", labels: [{ name: "codex-automerge" }], mergeable: true, mergeable_state: "clean" }
  const actor = { login: "chatgpt-codex-connector[bot]", id: 199175422, type: "Bot" }
  const app = { id: 1144995, slug: "chatgpt-codex-connector" }
  const comment = (id, body, created, updated = created) => ({ id, node_id: `node-${id}`, body,
    created_at: at(created), updated_at: at(updated), user: { ...actor }, performed_via_github_app: { ...app } })
  const marker = JSON.stringify({ repository: repo, pullRequestNumber: 42, headSha: sha, status: "completed" })
  const row = (kind, second) => `| 📝 **${kind} Review** | ✅ **Completed** <relative-time datetime="${at(second)}">${at(second)}</relative-time> | \`${sha.slice(0, 7)}\` | Manual request |`
  const comments = [
    comment(1, `<!-- codex-pull-request-review-summary -->\n<!-- codex-security-review:v1 ${marker} -->\n${row("Code", 5)}\n${row("Security", 9)}`, 0, 10),
    comment(2, `Codex Review: Didn't find any major issues.\n\n**Reviewed commit:** \`${sha.slice(0, 10)}\``, 4),
    comment(3, `Security review completed. No security issues were found in this pull request.\n\n**Reviewed commit:** \`${sha.slice(0, 10)}\``, 8),
  ]
  const bot = { __typename: "Bot", databaseId: 199175422 }
  const f = { pr, comments, reviews: [], threads: [], now: Date.parse(at(11)), calls: [], writes: [], canceled: [],
    checks: requiredChecks.map((name, id) => ({ id: id + 1, name, status: "completed", conclusion: "success", app: { slug: "github-actions" } })),
    alerts: [], metadata: { full_name: repo, default_branch: "main", private: true, fork: false, allow_auto_merge: true },
    branch: { protected: true, commit: { sha: base } },
  }
  f.nodes = () => f.comments.map(c => ({ databaseId: c.id, body: c.body, updatedAt: c.updated_at,
    lastEditedAt: c.id === 1 ? c.updated_at : null, author: bot, editor: c.id === 1 ? bot : null }))
  f.query = async body => {
    assert.ok(!body.query.includes("mutation"))
    if (body.query.includes("nodes(ids:")) return { data: { nodes: f.nodes() } }
    return { data: { repository: { pullRequest: { headRefOid: f.threadSHA || sha,
      reviewThreads: { nodes: f.threads, pageInfo: { hasNextPage: false, endCursor: null } } } } } }
  }
  f.get = async (path, options = {}) => {
    f.calls.push({ path, options })
    if (options.method === "PUT") { f.writes.push({ path, ...options }); return { merged: true, sha: "c".repeat(40) } }
    if (path === "graphql") return f.query(options.body)
    if (path === `repos/${repo}`) return structuredClone(f.metadata)
    if (path === `repos/${repo}/branches/main`) return structuredClone(f.branch)
    if (path.startsWith(`repos/${repo}/issues/42/comments?`)) return structuredClone(f.comments)
    if (path.startsWith(`repos/${repo}/pulls/42/reviews?`)) return structuredClone(f.reviews)
    if (path.includes("/git/ref/")) { assert.equal(options.missing, true); return f.shadow || null }
    if (path.includes("/check-runs?")) return { check_runs: structuredClone(f.checks) }
    if (path.includes("/code-scanning/alerts?")) return structuredClone(f.alerts)
    if (path === `repos/${repo}/commits/${sha.slice(0, 10)}`) return { sha: f.resolvedSHA || sha }
    if (path === `repos/${repo}/pulls/42`) return structuredClone(f.finalPR || f.pr)
    throw new Error(`Unexpected mock read: ${path}`)
  }
  f.verify = () => verifyPullRequestReviews(repo, f.pr, { get: f.get, query: f.query, now: f.now })
  f.cancel = args => { f.canceled.push(args); f.pr.auto_merge = null }
  f.merge = () => mergeReviewedPullRequest(repo, structuredClone(f.pr), { request: f.get,
    verifyReviews: (r, p, deps) => verifyPullRequestReviews(r, p, { ...deps, now: f.now }), cancelQueued: f.cancel, log: () => {} })
  return f
}

test("observed trusted clean reviews certify a complete SHA and pre-merge chronology", async () => {
  const f = fixture()
  assert.equal((await f.verify()).head, sha)
  f.pr.merged_at = at(11)
  f.pr.state = "closed"
  assert.equal((await f.verify()).head, sha)
  assert.equal(f.writes.length, 0)
})

test("missing, forged, stale, running, failed or malformed evidence always blocks", () => {
  const mutations = [
    f => { f.comments = [] },
    f => { f.comments[0].user.login = "someone-else" },
    f => { f.comments[0].user.id = 123 },
    f => { f.comments[0].user.type = "User" },
    f => { f.comments[0].performed_via_github_app.id = 123 },
    f => { f.comments.push({ ...f.comments[0], id: 4 }) },
    f => { f.comments[0].body = f.comments[0].body.replace('"status":"completed"', '"status":"running"') },
    f => { f.comments[0].body = f.comments[0].body.replace('"status":"completed"', '"status":"failed"') },
    f => { f.comments[0].body = f.comments[0].body.replace('"pullRequestNumber":42', '"pullRequestNumber":43') },
    f => { f.comments[0].body = f.comments[0].body.replace(repo, "outside/fork") },
    f => { f.comments[0].body = f.comments[0].body.replace(sha, "d".repeat(40)) },
    f => { f.comments[0].body = f.comments[0].body.replace('"headSha"', 'malformed') },
    f => { f.comments[0].body += '\n<!-- codex-security-review:v1 {} -->' },
    f => { f.comments[0].body = f.comments[0].body.replace("✅ **Completed**", "🔄 **Running**") },
    f => { f.comments[0].body = f.comments[0].body.replaceAll("Manual request", "Automatic review") },
    f => { f.comments[0].body = f.comments[0].body.replaceAll(at(5), "not-a-timestamp") },
    f => { f.comments[0].updated_at = at(1) },
    f => { f.pr.head.sha = "a".repeat(7) + "d".repeat(33); f.comments[0].body = f.comments[0].body.replace(sha, f.pr.head.sha) },
    f => { f.comments.splice(1, 1) },
    f => { f.comments[1].body = "👍" },
    f => { f.comments[1].body += `\n**Reviewed commit:** \`${sha}\`` },
    f => { f.comments[1].created_at = at(6) },
    f => { f.comments[1].node_id = null },
    f => { f.pr.merged_at = at(6) },
  ]
  for (const mutate of mutations) {
    const f = fixture(); mutate(f)
    assert.throws(() => reviewEvidence(repo, f.pr, f.comments, f.now), ReviewGateError)
  }
})

test("SHA resolution, GraphQL state, comment edits and unresolved reviews fail closed", async () => {
  const mutations = [
    f => { f.shadow = { ref: "refs/heads/shadow" } },
    f => { f.resolvedSHA = "d".repeat(40) },
    f => { f.threadSHA = "d".repeat(40) },
    f => { f.threads = [{ isResolved: false }] },
    f => { f.threads = [{}] },
    f => { f.reviews = [{ id: 4, user: { login: "reviewer" }, state: "CHANGES_REQUESTED" }] },
    f => { f.reviews = [{ id: 4, user: { login: "reviewer" }, state: "PENDING" }] },
    f => { f.reviews = [{ id: 4, user: { login: "reviewer" }, state: "UNKNOWN" }] },
    f => { f.comments.push(structuredClone(f.comments[2])) },
    f => { f.query = async () => ({ errors: [{ message: "forbidden" }] }) },
    f => { f.finalPR = { ...f.pr, head: { ...f.pr.head, sha: "d".repeat(40) } } },
    f => { const nodes = f.nodes(); nodes[1].body += "edited"; f.nodes = () => nodes },
    f => { const nodes = f.nodes(); nodes[1].lastEditedAt = at(4); nodes[1].editor = { __typename: "User", databaseId: 123 }; f.nodes = () => nodes },
    f => { const nodes = f.nodes(); nodes[0].editor.databaseId = 123; f.nodes = () => nodes },
    f => { f.nodes = () => [] },
  ]
  for (const mutate of mutations) {
    const f = fixture(); mutate(f)
    await assert.rejects(f.verify(), ReviewGateError)
    assert.equal(f.writes.length, 0)
  }
  const f = fixture(), get = f.get
  f.get = async (path, options) => {
    if (path.endsWith(`/commits/${sha.slice(0, 10)}`)) throw new Error("Ambiguous commit / service denied")
    return get(path, options)
  }
  await assert.rejects(f.verify(), /Ambiguous/)
})

test("pagination includes later comments and later unresolved threads; malformed pages stop", async () => {
  const f = fixture(), get = f.get
  const first = Array.from({ length: 100 }, (_, index) => ({ id: 100 + index, user: { login: "ordinary" }, body: "not evidence" }))
  f.get = (path, options) => path.includes("/comments?") && path.endsWith("page=1") ? first : get(path, options)
  assert.equal((await f.verify()).head, sha)
  const query = f.query
  f.query = async body => {
    if (body.query.includes("reviewThreads")) return { data: { repository: { pullRequest: { headRefOid: sha,
      reviewThreads: body.variables.after === null ? { nodes: Array.from({ length: 100 }, () => ({ isResolved: true })), pageInfo: { hasNextPage: true, endCursor: "next" } } :
        { nodes: [{ isResolved: false }], pageInfo: { hasNextPage: false } } } } } }
    return query(body)
  }
  await assert.rejects(f.verify(), /Unresolved/)
  f.get = async () => ({ invalid: "page" })
  await assert.rejects(f.verify(), /Invalid review evidence page/)
})

test("one synchronous protected merge uses all gates and never creates auto-merge", async () => {
  const f = fixture()
  assert.equal(await f.merge(), true)
  assert.deepEqual(f.writes, [{ path: `repos/${repo}/pulls/42/merge`, method: "PUT", body: { sha, merge_method: "squash" } }])
  assert.deepEqual(f.canceled, [])
  assert.equal(f.calls.some(call => JSON.stringify(call.options).includes("enablePullRequestAutoMerge")), false)
})

test("queued legacy permission is revoked before pending or changed-head checks", async () => {
  for (const scenario of ["pending", "label", "draft", "changed"]) {
    const f = fixture(), expected = structuredClone(f.pr)
    f.pr.auto_merge = { enabled: true }
    if (scenario === "pending") f.checks[0].conclusion = "failure"
    if (scenario === "label") f.pr.labels = []
    if (scenario === "draft") f.pr.draft = true
    if (scenario === "changed") f.pr.head.sha = "d".repeat(40)
    assert.equal(await mergeReviewedPullRequest(repo, expected, { request: f.get, cancelQueued: f.cancel, log: () => {} }), false)
    assert.deepEqual(f.canceled, [["pr", "merge", "42", "--repo", repo, "--disable-auto"]])
    assert.deepEqual(f.writes, [])
  }
})

test("failed checks, reviews, alerts, changed base and mergeability prevent every merge write", async () => {
  const mutations = [
    f => { f.checks[0].status = "in_progress" },
    f => { f.checks[0].app.slug = "forged" },
    f => { f.checks.pop() },
    f => { f.comments = [] },
    f => { f.alerts = [{ number: 1 }] },
    f => { f.pr.mergeable = false },
    f => { f.pr.mergeable_state = "blocked" },
    f => { const query = f.query; f.query = async body => { const result = await query(body); f.branch.commit.sha = "d".repeat(40); return result } },
    f => { const query = f.query; f.query = async body => { const result = await query(body); f.pr.labels = []; return result } },
    f => { const query = f.query; f.query = async body => { const result = await query(body); f.checks[0].status = "in_progress"; return result } },
  ]
  for (const mutate of mutations) {
    const f = fixture(); mutate(f)
    assert.equal(await f.merge(), false)
    assert.deepEqual(f.writes, [])
  }
  const f = fixture(); f.metadata.private = false
  await assert.rejects(f.merge(), /protected standalone/)
  assert.deepEqual(f.writes, [])
})

test("metadata reconciliation never opts in forks, dependency bots or bot release PRs", () => {
  const { pr } = fixture()
  assert.equal(eligibleEngineeringPR(repo, pr), true)
  for (const author_association of ["NONE", "CONTRIBUTOR", "FIRST_TIMER"]) assert.equal(eligibleEngineeringPR(repo, { ...pr, author_association }), false)
  assert.equal(eligibleEngineeringPR(repo, { ...pr, head: { repo: { full_name: "outside/fork" } } }), false)
})

test("concurrent native queues are revoked and inconsistent API identities never mutate a PR", async () => {
  const f = fixture(), query = f.query
  f.query = async body => { const result = await query(body); f.pr.auto_merge = { enabled: true }; return result }
  assert.equal(await f.merge(), false)
  assert.equal(f.canceled.length, 1)
  assert.deepEqual(f.writes, [])
  const wrong = fixture()
  wrong.finalPR = { ...wrong.pr, number: 43, auto_merge: { enabled: true } }
  await assert.rejects(wrong.merge(), /inconsistent PR identity/)
  assert.deepEqual(wrong.canceled, [])
  assert.deepEqual(wrong.writes, [])
})

test("a stale ten-character collision, service error or native SHA conflict cannot bypass reviews", async () => {
  const f = fixture(), original = f.pr.head.sha
  f.pr.head.sha = original.slice(0, 10) + "d".repeat(30)
  f.comments[0].body = f.comments[0].body.replace(original, f.pr.head.sha)
  await assert.rejects(f.verify(), /complete expected SHA|full expected SHA/)
  const denied = fixture(), get = denied.get
  denied.get = (path, options) => path.includes("/comments?") ? Promise.reject(new Error("HTTP 403")) : get(path, options)
  await assert.rejects(denied.merge(), /HTTP 403/)
  assert.deepEqual(denied.writes, [])
  const conflict = fixture(), request = conflict.get
  conflict.get = (path, options) => options?.method === "PUT" ? Promise.reject(new Error("HTTP 409 SHA conflict")) : request(path, options)
  await assert.rejects(conflict.merge(), /HTTP 409/)
  assert.deepEqual(conflict.canceled, [])
  assert.equal(conflict.calls.some(call => /merge-async/.test(call.path)), false)
})

test("all automated merge paths use protected-main code and the same review interlock", () => {
  const workflow = readFileSync(new URL("../.github/workflows/automerge.yml", import.meta.url), "utf8")
  const prepare = readFileSync(new URL("./release-prepare.mjs", import.meta.url), "utf8")
  const publish = readFileSync(new URL("./release-publish.mjs", import.meta.url), "utf8")
  assert.match(workflow, /pull_request_target:/)
  assert.match(workflow, /ref: main\s+persist-credentials: false/)
  assert.doesNotMatch(workflow, /allow-unsafe-pr-checkout|head\.sha|refs\/pull|npm install|download-artifact|--auto|--admin/)
  assert.match(workflow, /run: node scripts\/automerge\.mjs/)
  assert.match(prepare, /if \(created\) await api\([^\n]+labels/)
  assert.match(prepare, /await mergeReviewedPullRequest/)
  assert.match(publish, /await verifyPullRequestReviews/)
  assert.match(publish, /const finalSource = await source\(\)/)
})
