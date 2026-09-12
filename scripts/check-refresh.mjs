import { api } from "./release-lib.mjs"

const shaPattern = /^[a-f0-9]{40}$/
const releaseBranchPattern = /^release\/v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/

async function workflowRuns(repo, workflow, branch, request) {
  const result = await request(`repos/${repo}/actions/workflows/${workflow}/runs?branch=${encodeURIComponent(branch)}&per_page=20`)
  if (!Array.isArray(result?.workflow_runs)) throw new Error(`GitHub returned malformed ${workflow} workflow runs`)
  return result.workflow_runs
}

function completedWithoutExecution(run) {
  return run?.status === "completed" && run?.conclusion === "action_required"
}

async function hasExactRun(repo, workflow, branch, sha, request) {
  return (await workflowRuns(repo, workflow, branch, request)).some((run) => run?.head_sha === sha && !completedWithoutExecution(run))
}

async function validateTarget(repo, branch, request) {
  if (branch !== "main" && !releaseBranchPattern.test(branch)) throw new Error("check refresh is limited to protected main or generated release branches")
  const ref = await request(`repos/${repo}/git/ref/heads/${branch}`, { missing: true })
  const sha = ref?.object?.sha
  if (!shaPattern.test(sha || "")) throw new Error("check refresh target branch has no exact commit SHA")

  if (branch === "main") {
    const metadata = await request(`repos/${repo}`)
    const protectedBranch = await request(`repos/${repo}/branches/main`)
    if (metadata?.default_branch !== "main" || metadata?.private !== false || metadata?.fork !== false || protectedBranch?.protected !== true || protectedBranch?.commit?.sha !== sha) {
      throw new Error("main check refresh requires the current protected standalone public main commit")
    }
    return sha
  }

  const owner = repo.split("/")[0]
  const pulls = await request(`repos/${repo}/pulls?state=open&base=main&head=${encodeURIComponent(`${owner}:${branch}`)}&per_page=10`)
  if (!Array.isArray(pulls) || pulls.length !== 1) throw new Error("generated release check refresh requires exactly one open release PR")
  const pr = pulls[0]
  if (pr?.draft !== false || pr?.base?.ref !== "main" || pr?.head?.ref !== branch || pr?.head?.sha !== sha || pr?.head?.repo?.full_name !== repo || pr?.title !== `release: Darkphish ${branch.slice("release/v".length)}`) {
    throw new Error("generated release check refresh target does not match the trusted release PR shape")
  }
  return sha
}

export async function dispatchChecks(repo, branch, { request = api } = {}) {
  const sha = await validateTarget(repo, branch, request)
  let dispatched = false

  if (!await hasExactRun(repo, "ci.yml", branch, sha, request)) {
    await request(`repos/${repo}/actions/workflows/ci.yml/dispatches`, { method: "POST", body: { ref: branch } })
    dispatched = true
  }

  if (branch === "main") {
    if (!await hasExactRun(repo, "codeql.yml", branch, sha, request)) {
      await request(`repos/${repo}/dispatches`, { method: "POST", body: { event_type: "protected-main-codeql", client_payload: { sha } } })
      dispatched = true
    }
  } else {
    const canonical = await hasExactRun(repo, "codeql.yml", branch, sha, request)
    const required = await request(`repos/${repo}/commits/${sha}/check-runs?filter=latest&per_page=100`)
    const names = new Set((required?.check_runs || []).filter((run) => run?.app?.slug === "github-actions" && run?.status === "completed" && run?.conclusion === "success").map((run) => run.name))
    if (!canonical && (!names.has("CodeQL (go)") || !names.has("CodeQL (javascript-typescript)"))) {
      await request(`repos/${repo}/dispatches`, { method: "POST", body: { event_type: "generated-release-codeql", client_payload: { branch, sha } } })
      dispatched = true
    }
  }

  return { sha, dispatched }
}
