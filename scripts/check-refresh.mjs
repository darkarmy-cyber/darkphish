import { api, pages } from "./release-lib.mjs"

const shaPattern = /^[a-f0-9]{40}$/
const releaseBranchPattern = /^release\/v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/
const requiredCodeQLChecks = ["CodeQL (go)", "CodeQL (javascript-typescript)"]

async function workflowRuns(repo, workflow, branch, request) {
  return pages(`repos/${repo}/actions/workflows/${workflow}/runs?branch=${encodeURIComponent(branch)}`, "workflow_runs", request)
}

function completedWithoutExecution(run) {
  return run?.status === "completed" && run?.conclusion === "action_required"
}

function hasAnyExactRun(runs, sha) {
  return runs.some((run) => run?.head_sha === sha)
}

function needsGeneratedReleaseRefresh(runs, sha) {
  const exact = runs.filter((run) => run?.head_sha === sha)
  return exact.length === 0 || exact.every(completedWithoutExecution)
}

function generatedReleaseCodeQLState(checks) {
  const matching = (checks || []).filter((check) =>
    check?.app?.slug === "github-actions" && requiredCodeQLChecks.includes(check?.name) && Number.isSafeInteger(check?.id))
  const present = new Set(matching.map((check) => check.name))

  // Fail closed across the complete exact-head history. Once a required CodeQL
  // check exists and is non-successful, a later successful check with the same
  // name must never erase that evidence or authorize another recovery dispatch.
  const blocking = matching.some((check) => check.status !== "completed" || check.conclusion !== "success")
  const missing = requiredCodeQLChecks.some((name) => !present.has(name))
  return { blocking, missing }
}

function refreshMarker(kind, sha) {
  if (!/^(ci|codeql)$/.test(kind) || !shaPattern.test(sha)) throw new Error("invalid generated release refresh marker")
  return `<!-- darkphish-check-refresh:${kind}:${sha} -->`
}

function assertSameReleasePull(repo, expected, actual, sha) {
  if (!Number.isSafeInteger(actual?.number) || actual.number !== expected.number || actual?.draft !== false || actual?.base?.ref !== "main" || actual?.head?.repo?.full_name !== repo || actual?.head?.ref !== expected.head.ref || actual?.head?.sha !== sha || actual?.title !== expected.title) {
    throw new Error("generated release pull request changed while recording check refresh state")
  }
}

async function markRefreshRequested(repo, pr, kind, sha, request) {
  const marker = refreshMarker(kind, sha)
  if (typeof pr.body === "string" && pr.body.includes(marker)) return false

  const current = await request(`repos/${repo}/pulls/${pr.number}`)
  assertSameReleasePull(repo, pr, current, sha)
  const body = typeof current.body === "string" ? current.body : ""
  if (body.includes(marker)) {
    pr.body = body
    return false
  }

  // Persist the one-shot marker before dispatch. If the dispatch subsequently fails,
  // recovery stays fail-closed instead of amplifying into an unbounded retry loop.
  const nextBody = `${body}${body && !body.endsWith("\n") ? "\n" : ""}\n${marker}`
  const updated = await request(`repos/${repo}/pulls/${pr.number}`, { method: "PATCH", body: { body: nextBody } })
  assertSameReleasePull(repo, pr, updated, sha)
  if (typeof updated.body !== "string" || !updated.body.includes(marker)) throw new Error("generated release refresh marker was not persisted")
  pr.body = updated.body
  return true
}

async function validateTarget(repo, branch, request) {
  if (branch !== "main" && !releaseBranchPattern.test(branch)) throw new Error("check refresh is limited to protected main or generated release branches")
  const ref = await request(`repos/${repo}/git/ref/heads/${branch}`, { missing: true })
  const sha = ref?.object?.sha
  if (!shaPattern.test(sha || "")) throw new Error("check refresh target branch has no exact commit SHA")

  if (branch === "main") {
    const metadata = await request(`repos/${repo}`)
    const protectedBranch = await request(`repos/${repo}/branches/main`)
    if (metadata?.default_branch !== "main" || metadata.private !== false || metadata.fork !== false || protectedBranch?.protected !== true || protectedBranch?.commit?.sha !== sha) {
      throw new Error("main check refresh requires the current protected standalone public main commit")
    }
    return { sha, pr: null }
  }

  const owner = repo.split("/")[0]
  const pulls = await request(`repos/${repo}/pulls?state=open&base=main&head=${encodeURIComponent(`${owner}:${branch}`)}&per_page=10`)
  if (!Array.isArray(pulls) || pulls.length !== 1) throw new Error("generated release check refresh requires exactly one open release PR")
  const pr = pulls[0]
  if (!Number.isSafeInteger(pr?.number) || pr.number < 1 || pr?.draft !== false || pr?.base?.ref !== "main" || pr?.head?.ref !== branch || pr?.head?.sha !== sha || pr?.head?.repo?.full_name !== repo || pr?.title !== `release: Darkphish ${branch.slice("release/v".length)}`) {
    throw new Error("generated release check refresh target does not match the trusted release PR shape")
  }
  return { sha, pr }
}

export async function dispatchChecks(repo, branch, { request = api } = {}) {
  const { sha, pr } = await validateTarget(repo, branch, request)
  let dispatched = false

  const ciRuns = await workflowRuns(repo, "ci.yml", branch, request)
  if (branch === "main") {
    if (!hasAnyExactRun(ciRuns, sha)) {
      await request(`repos/${repo}/actions/workflows/ci.yml/dispatches`, { method: "POST", body: { ref: branch } })
      dispatched = true
    }
  } else if (needsGeneratedReleaseRefresh(ciRuns, sha) && await markRefreshRequested(repo, pr, "ci", sha, request)) {
    await request(`repos/${repo}/actions/workflows/ci.yml/dispatches`, { method: "POST", body: { ref: branch } })
    dispatched = true
  }

  const codeqlRuns = await workflowRuns(repo, "codeql.yml", branch, request)
  if (branch === "main") {
    if (!hasAnyExactRun(codeqlRuns, sha)) {
      await request(`repos/${repo}/dispatches`, { method: "POST", body: { event_type: "protected-main-codeql", client_payload: { sha } } })
      dispatched = true
    }
  } else if (needsGeneratedReleaseRefresh(codeqlRuns, sha)) {
    const required = await pages(`repos/${repo}/commits/${sha}/check-runs?filter=latest`, "check_runs", request)
    const state = generatedReleaseCodeQLState(required)
    // A present non-successful required check is authoritative and blocking. Never
    // refresh it away with a newer success. Only recover genuinely absent checks.
    if (!state.blocking && state.missing && await markRefreshRequested(repo, pr, "codeql", sha, request)) {
      await request(`repos/${repo}/dispatches`, { method: "POST", body: { event_type: "generated-release-codeql", client_payload: { branch, sha } } })
      dispatched = true
    }
  }

  return { sha, dispatched }
}