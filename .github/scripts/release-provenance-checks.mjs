const actionsBot = (actor) => actor?.login === "github-actions[bot]" && actor?.type === "Bot" && actor?.id === 41898282
const time = (value) => {
  const parsed = Date.parse(value || "")
  return Number.isFinite(parsed) ? parsed : null
}

export function canonicalMainCheckRun(run, name, source, before = Infinity) {
  const path = name === "CI" ? ".github/workflows/ci.yml" : name === "CodeQL" ? ".github/workflows/codeql.yml" : null
  const events = name === "CI" ? ["push", "workflow_dispatch"] : name === "CodeQL" ? ["push", "repository_dispatch"] : []
  const completed = time(run?.updated_at)
  return Boolean(path && events.includes(run?.event) && (run.event === "push" || actionsBot(run.actor)) &&
    run.name === name && run.path === path && run.head_branch === "main" && run.head_sha === source &&
    run.status === "completed" && run.conclusion === "success" && completed !== null && completed <= before)
}

export function nativeReleaseAttemptStart(run, metadata) {
  const created = time(run?.created_at), started = time(metadata?.started_at), completed = time(metadata?.completed_at)
  if (!Number.isSafeInteger(run?.id) || run.id < 1 || !Number.isSafeInteger(run?.run_attempt) || run.run_attempt < 1 ||
    metadata?.name !== "metadata" || metadata?.run_id !== run.id || metadata.run_attempt !== run.run_attempt ||
    metadata.head_sha !== run.head_sha || created === null || started === null || completed === null ||
    started < created || completed < started) throw new Error("native release metadata is not bound to its completed run attempt")
  return started
}
