import { api } from "./release-lib.mjs"

const shaPattern = /^[a-f0-9]{40}$/
const releaseBranchPattern = /^release\/v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/
const releaseHeadConvergenceAttempts = 6
const releaseHeadConvergenceDelayMs = 1000

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

async function workflowRuns(repo, workflow, branch, request) {
  const result = await request(`repos/${repo}/actions/workflows/${workflow}/runs?branch=${encodeURIComponent(branch)}&per_page=20`)
  if (!Array.isArray(result?.workflow_runs)) throw new Error(`GitHub returned malformed ${workflow} workflow runs`)
  return result.workflow_runs
}

async function hasExactRun(repo, workflow, branch, sha, request) {
  return (await workflowRuns(repo, workflow, branch, request)).some((run) => run?.head_sha === sha)
}

async function exactBranchSHA(repo, branch, expectedSHA, request) {
  const ref = await request(`repos/${repo}/git/ref/heads/${branch}`, { missing: true })
  const sha = ref?.object?.sha
  if (!shaPattern.test(sha || "")) throw new Error("check refresh target branch has no exact commit SHA")
  if (expectedSHA && sha !== expectedSHA) throw new Error("generated release branch moved during pull-request head convergence")
  return sha
}

function releasePullIdentity(pr) {
  if (!Number.isInteger(pr?.number) || !Number.isInteger(pr?.id) || typeof pr?.node_id !== "string" || !pr.node_id) {
    throw new Error("generated release PR is missing immutable identity metadata")
  }
  return { number: pr.number, id: pr.id, nodeID: pr.node_id }
}

function sameReleasePullIdentity(pr, identity) {
  return pr?.number === identity.number && pr?.id === identity.id && pr?.node_id === identity.nodeID
}

async function trustedReleasePull(repo, branch, sha, request) {
  const owner = repo.split("/")[0]
  let identity = null
  for (let attempt = 1; attempt <= releaseHeadConvergenceAttempts; attempt += 1) {
    await exactBranchSHA(repo, branch, sha, request)
    const pulls = await request(`repos/${repo}/pulls?state=open&base=main&head=${encodeURIComponent(`${owner}:${branch}`)}&per_page=10`)
    if (!Array.isArray(pulls) || pulls.length !== 1) throw new Error("generated release check refresh requires exactly one open release PR")
    const pr = pulls[0]
    if (pr?.draft !== false || pr?.base?.ref !== "main" || pr?.head?.ref !== branch || pr?.head?.repo?.full_name !== repo || pr?.title !== `release: Darkphish ${branch.slice("release/v".length)}`) {
      throw new Error("generated release check refresh target does not match the trusted release PR shape")
    }
    if (!identity) identity = releasePullIdentity(pr)
    else if (!sameReleasePullIdentity(pr, identity)) throw new Error("generated release PR identity changed during head convergence")
    if (pr?.head?.sha === sha) {
      await exactBranchSHA(repo, branch, sha, request)
      return pr
    }
    if (attempt < releaseHeadConvergenceAttempts) await sleep(releaseHeadConvergenceDelayMs)
  }
  throw new Error("generated release PR head did not converge to the exact branch SHA")
}

async function validateTarget(repo, branch, request) {
  if (branch !== "main" && !releaseBranchPattern.test(branch)) throw new Error("check refresh is limited to protected main or generated release branches")
  const sha = await exactBranchSHA(repo, branch, null, request)

  if (branch === "main") {
    const metadata = await request(`repos/${repo}`)
    const protectedBranch = await request(`repos/${repo}/branches/main`)
    if (metadata?.default_branch !== "main" || metadata?.private !== false || metadata?.fork !== false || protectedBranch?.protected !== true || protectedBranch?.commit?.sha !== sha) {
      throw new Error("main check refresh requires the current protected standalone public main commit")
    }
    return sha
  }

  await trustedReleasePull(repo, branch, sha, request)
  return sha
}

export async function dispatchChecks(repo, branch, { request = api } = {}) {
  const sha = await validateTarget(repo, branch, request)
  let dispatched = false

  if (!await hasExactRun(repo, "ci.yml", branch, sha, request)) {
    await exactBranchSHA(repo, branch, sha, request)
    await request(`repos/${repo}/actions/workflows/ci.yml/dispatches`, { method: "POST", body: { ref: branch } })
    dispatched = true
  }

  if (branch === "main") {
    if (!await hasExactRun(repo, "codeql.yml", branch, sha, request)) {
      await exactBranchSHA(repo, branch, sha, request)
      await request(`repos/${repo}/dispatches`, { method: "POST", body: { event_type: "protected-main-codeql", client_payload: { sha } } })
      dispatched = true
    }
  } else {
    const canonical = await hasExactRun(repo, "codeql.yml", branch, sha, request)
    const required = await request(`repos/${repo}/commits/${sha}/check-runs?filter=latest&per_page=100`)
    const names = new Set((required?.check_runs || []).filter((run) => run?.app?.slug === "github-actions" && run?.status === "completed" && run?.conclusion === "success").map((run) => run.name))
    if (!canonical && (!names.has("CodeQL (go)") || !names.has("CodeQL (javascript-typescript)"))) {
      await exactBranchSHA(repo, branch, sha, request)
      await request(`repos/${repo}/dispatches`, { method: "POST", body: { event_type: "generated-release-codeql", client_payload: { branch, sha } } })
      dispatched = true
    }
  }

  return { sha, dispatched }
}
