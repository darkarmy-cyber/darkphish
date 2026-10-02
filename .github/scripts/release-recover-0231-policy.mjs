const repository = "darkarmy-cyber/darkphish"
const version = "0.23.1"
const tag = "v0.23.1"
const source = "86cb7173c553d68bc3afac204948aa923a32bc83"

export function auditedTagless0231Source(candidate) {
  const pr = candidate?.pr
  return candidate?.repository === repository && candidate.version === version && candidate.tag === tag &&
    pr?.number === 181 && pr.state === "closed" && pr.draft === false &&
    pr.title === "release: Darkphish 0.23.1" && pr.merged_at === "2026-10-02T12:24:22Z" &&
    pr.merge_commit_sha === source && pr.merged_by?.login === "oliverkko" && pr.merged_by?.id === 309485696 &&
    pr.base?.ref === "main" && pr.base?.sha === "443d247b1636ea8e46c7339378174d3c88a0abf5" &&
    pr.head?.ref === "release/v0.23.1" && pr.head?.sha === "5f8b6de5a6ed4076b987747c32d615b5b9bd180a" &&
    pr.head?.repo?.full_name === repository ? source : null
}

export function audited0231MetadataFailure(candidate) {
  const { run, jobs } = candidate || {}
  if (candidate?.repository !== repository || candidate.source !== source ||
    run?.id !== 37009036246 || run.run_number !== 474 || run.run_attempt !== 2 ||
    run.workflow_id !== 351159930 || run.name !== "Native release" ||
    run.path !== ".github/workflows/release.yml" || run.event !== "schedule" ||
    run.head_branch !== "main" || run.head_sha !== source || run.status !== "completed" ||
    run.conclusion !== "failure" || run.created_at !== "2026-10-02T12:49:30Z" ||
    run.updated_at !== "2026-10-02T12:54:52Z" ||
    run.actor?.login !== "github-actions[bot]" || run.actor?.id !== 41898282 ||
    run.actor?.type !== "Bot" || run.triggering_actor?.login !== "oliverkko" ||
    run.triggering_actor?.id !== 309485696 || !Array.isArray(jobs) || jobs.length !== 5) return false

  const expectedJobs = new Map([
    [110845725168, ["metadata", "failure", "2026-10-02T12:54:34Z", "2026-10-02T12:54:51Z"]],
    [110845838722, ["verify", "skipped", "2026-10-02T12:54:51Z", "2026-10-02T12:54:51Z"]],
    [110845839460, ["binaries", "skipped", "2026-10-02T12:54:51Z", "2026-10-02T12:54:51Z"]],
    [110845839732, ["audit-smoke", "skipped", "2026-10-02T12:54:51Z", "2026-10-02T12:54:51Z"]],
    [110845839869, ["publish", "skipped", "2026-10-02T12:54:51Z", "2026-10-02T12:54:51Z"]],
  ])
  if (new Set(jobs.map(job => job?.id)).size !== expectedJobs.size) return false
  for (const job of jobs) {
    const expected = expectedJobs.get(job?.id)
    if (!expected || job.run_id !== run.id || job.run_attempt !== 2 ||
      job.head_sha !== source || job.status !== "completed" ||
      job.name !== expected[0] || job.conclusion !== expected[1] ||
      job.started_at !== expected[2] || job.completed_at !== expected[3]) return false
  }
  const metadata = jobs.find(job => job.id === 110845725168)
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
