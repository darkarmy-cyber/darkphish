const repository = "darkarmy-cyber/darkphish"
const version = "0.24.0"
const tag = "v0.24.0"
const source = "3f20a49ee02bddff90f03db67edeae0e236cf55f"

export function auditedTagless0240Source(candidate) {
  const pr = candidate?.pr
  return candidate?.repository === repository && candidate.version === version && candidate.tag === tag &&
    pr?.number === 186 && pr.state === "closed" && pr.draft === false &&
    pr.title === "release: Darkphish 0.24.0" && pr.merged_at === "2026-10-02T16:39:39Z" &&
    pr.merge_commit_sha === source && pr.merged_by?.login === "oliverkko" && pr.merged_by?.id === 309485696 &&
    pr.base?.ref === "main" && pr.base?.sha === "01f38cccbb8e466239be360cd5682a78a1620560" &&
    pr.head?.ref === "release/v0.24.0" && pr.head?.sha === "7b808b9ab02b46c6f0ea80b6c382bd1a7acdabad" &&
    pr.head?.repo?.full_name === repository ? source : null
}

export function audited0240MetadataFailure(candidate) {
  const { run, jobs } = candidate || {}
  if (candidate?.repository !== repository || candidate.source !== source ||
    run?.id !== 37035934324 || run.run_number !== 484 || run.run_attempt !== 1 ||
    run.workflow_id !== 351159930 || run.name !== "Native release" ||
    run.path !== ".github/workflows/release.yml" || run.event !== "workflow_run" ||
    run.head_branch !== "main" || run.head_sha !== source || run.status !== "completed" ||
    run.conclusion !== "failure" || run.created_at !== "2026-10-02T16:44:09Z" ||
    run.updated_at !== "2026-10-02T16:49:29Z" ||
    run.actor?.login !== "oliverkko" || run.actor?.id !== 309485696 ||
    run.actor?.type !== "User" || !Array.isArray(jobs) || jobs.length !== 5) return false

  const expectedJobs = new Map([
    [110935919656, ["metadata", "failure", "2026-10-02T16:49:13Z", "2026-10-02T16:49:28Z"]],
    [110936033864, ["verify", "skipped", "2026-10-02T16:49:29Z", "2026-10-02T16:49:28Z"]],
    [110936034987, ["binaries", "skipped", "2026-10-02T16:49:29Z", "2026-10-02T16:49:28Z"]],
    [110936035915, ["audit-smoke", "skipped", "2026-10-02T16:49:29Z", "2026-10-02T16:49:29Z"]],
    [110936036722, ["publish", "skipped", "2026-10-02T16:49:29Z", "2026-10-02T16:49:29Z"]],
  ])
  if (new Set(jobs.map(job => job?.id)).size !== expectedJobs.size) return false
  for (const job of jobs) {
    const expected = expectedJobs.get(job?.id)
    if (!expected || job.run_id !== run.id || job.run_attempt !== 1 ||
      job.head_sha !== source || job.status !== "completed" ||
      job.name !== expected[0] || job.conclusion !== expected[1] ||
      job.started_at !== expected[2] || job.completed_at !== expected[3]) return false
  }
  const metadata = jobs.find(job => job.id === 110935919656)
  const expectedSteps = [
    [1,"Set up job","success"],[2,"Run actions/checkout@v7","success"],[3,"Run actions/setup-node@v7","success"],
    [4,"Run node scripts/changelog.mjs validate","success"],[5,"Enforce release normalization hold","success"],
    [6,"Run node scripts/release-publish.mjs metadata","failure"],[11,"Post Run actions/setup-node@v7","skipped"],
    [12,"Post Run actions/checkout@v7","success"],[13,"Complete job","success"],
  ]
  return Array.isArray(metadata?.steps) && metadata.steps.length === expectedSteps.length &&
    expectedSteps.every(([number,name,conclusion],i) => {
      const step=metadata.steps[i]
      return step?.number===number && step.name===name && step.status==="completed" && step.conclusion===conclusion
    })
}
