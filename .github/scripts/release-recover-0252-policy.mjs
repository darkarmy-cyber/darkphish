const repository = "darkarmy-cyber/darkphish"
const version = "0.25.2"
const tag = "v0.25.2"
const source = "eb0fe18b9fde19fea159961cb9e3c863cb58b437"

export function auditedTagless0252Source(candidate) {
  const pr = candidate?.pr
  return candidate?.repository === repository && candidate.version === version && candidate.tag === tag &&
    pr?.number === 206 && pr.state === "closed" && pr.draft === false &&
    pr.title === "release: Darkphish 0.25.2" && pr.merged_at === "2026-10-03T20:45:16Z" &&
    pr.merge_commit_sha === source && pr.merged_by?.login === "oliverkko" && pr.merged_by?.id === 309485696 &&
    pr.base?.ref === "main" && pr.base?.sha === "a9bae16aa396edd3f19856142abbd763add09674" &&
    pr.head?.ref === "release/v0.25.2" && pr.head?.sha === "5e10e7f894b62f84fbe0685f8cffd631bda328ed" &&
    pr.head?.repo?.full_name === repository ? source : null
}

export function audited0252MetadataFailure(candidate) {
  const { run, jobs } = candidate || {}
  if (candidate?.repository !== repository || candidate.source !== source ||
    run?.id !== 37152934161 || run.run_number !== 515 || run.run_attempt !== 1 ||
    run.workflow_id !== 351159930 || run.name !== "Native release" ||
    run.path !== ".github/workflows/release.yml" || run.event !== "workflow_run" ||
    run.head_branch !== "main" || run.head_sha !== source || run.status !== "completed" ||
    run.conclusion !== "failure" || run.created_at !== "2026-10-03T20:49:48Z" ||
    run.updated_at !== "2026-10-03T20:50:03Z" ||
    run.actor?.login !== "oliverkko" || run.actor?.id !== 309485696 ||
    run.actor?.type !== "User" || !Array.isArray(jobs) || jobs.length !== 5) return false

  const expectedJobs = new Map([
    [111290349984, ["metadata", "failure"]],
    [111290392359, ["verify", "skipped"]],
    [111290392376, ["binaries", "skipped"]],
    [111290392792, ["audit-smoke", "skipped"]],
    [111290392908, ["publish", "skipped"]],
  ])
  if (new Set(jobs.map(job => job?.id)).size !== expectedJobs.size) return false
  for (const job of jobs) {
    const expected = expectedJobs.get(job?.id)
    if (!expected || job.run_id !== run.id || job.run_attempt !== 1 ||
      job.head_sha !== source || job.status !== "completed" ||
      job.name !== expected[0] || job.conclusion !== expected[1]) return false
  }
  const metadata = jobs.find(job => job.id === 111290349984)
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
