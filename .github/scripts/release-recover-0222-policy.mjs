const repository = "darkarmy-cyber/darkphish"
const source = "743633afd812980852480cf746a168c75814cde5"

export function audited0222MetadataFailure(candidate) {
  const { run, jobs } = candidate || {}
  if (candidate?.repository !== repository || candidate.source !== source ||
    run?.id !== 36717602969 || run.run_number !== 432 || run.run_attempt !== 2 ||
    run.workflow_id !== 351159930 || run.name !== "Native release" ||
    run.path !== ".github/workflows/release.yml" || run.event !== "workflow_run" ||
    run.head_branch !== "main" || run.head_sha !== source || run.status !== "completed" ||
    run.conclusion !== "failure" || run.created_at !== "2026-09-30T12:52:13Z" ||
    run.updated_at !== "2026-09-30T12:58:36Z" ||
    run.actor?.login !== "oliverkko" || run.actor?.id !== 309485696 ||
    run.actor?.type !== "User" || !Array.isArray(jobs) || jobs.length !== 5) return false

  const expectedJobs = new Map([
    [109896607865, ["metadata", "failure", "2026-09-30T12:58:16Z", "2026-09-30T12:58:34Z"]],
    [109896762661, ["verify", "skipped", "2026-09-30T12:58:35Z", "2026-09-30T12:58:35Z"]],
    [109896762934, ["binaries", "skipped", "2026-09-30T12:58:35Z", "2026-09-30T12:58:35Z"]],
    [109896763770, ["audit-smoke", "skipped", "2026-09-30T12:58:35Z", "2026-09-30T12:58:35Z"]],
    [109896765002, ["publish", "skipped", "2026-09-30T12:58:35Z", "2026-09-30T12:58:35Z"]],
  ])
  if (new Set(jobs.map(job => job?.id)).size !== expectedJobs.size) return false
  for (const job of jobs) {
    const expected = expectedJobs.get(job?.id)
    if (!expected || job.run_id !== run.id || job.run_attempt !== 2 ||
      job.head_sha !== source || job.status !== "completed" ||
      job.name !== expected[0] || job.conclusion !== expected[1] ||
      job.started_at !== expected[2] || job.completed_at !== expected[3]) return false
  }
  const metadata = jobs.find((job) => job.id === 109896607865)
  const steps = metadata?.steps
  if (!Array.isArray(steps) || steps.length !== 9) return false
  const expectedSteps = [
    [1, "Set up job", "success"],
    [2, "Run actions/checkout@v7", "success"],
    [3, "Run actions/setup-node@v7", "success"],
    [4, "Run node scripts/changelog.mjs validate", "success"],
    [5, "Enforce release normalization hold", "success"],
    [6, "Run node scripts/release-publish.mjs metadata", "failure"],
    [11, "Post Run actions/setup-node@v7", "skipped"],
    [12, "Post Run actions/checkout@v7", "success"],
    [13, "Complete job", "success"],
  ]
  return expectedSteps.every(([number, name, conclusion], index) => {
    const step = steps[index]
    return step?.number === number && step.name === name &&
      step.status === "completed" && step.conclusion === conclusion
  })
}
