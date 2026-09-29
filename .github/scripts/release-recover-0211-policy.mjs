const repository = "darkarmy-cyber/darkphish"
const version = "0.21.1"
const tag = "v0.21.1"
const source = "51f3c3b56df05d8a6fd165d227cb78d9e46ae821"

export function auditedTagless0211Source(candidate) {
  const pr = candidate?.pr
  return candidate?.repository === repository && candidate.version === version && candidate.tag === tag &&
    pr?.number === 128 && pr.state === "closed" && pr.draft === false &&
    pr.title === "release: Darkphish 0.21.1" && pr.merged_at === "2026-09-29T10:41:54Z" &&
    pr.merge_commit_sha === source && pr.base?.ref === "main" &&
    pr.base?.sha === "e9512fc4c564dabdc098b3bf44d3748c355d4d15" &&
    pr.head?.ref === "release/v0.21.1" && pr.head?.sha === "e5a4f4713cbe25d05d641c8b22e8e83da3ea69a7" &&
    pr.head?.repo?.full_name === repository ? source : null
}

export function audited0211MetadataFailure(candidate) {
  const { run, jobs } = candidate || {}
  if (candidate?.repository !== repository || candidate.source !== source ||
    run?.id !== 36557608515 || run.run_number !== 374 || run.run_attempt !== 2 ||
    run.workflow_id !== 351159930 || run.name !== "Native release" ||
    run.path !== ".github/workflows/release.yml" || run.event !== "workflow_run" ||
    run.head_branch !== "main" || run.head_sha !== source || run.status !== "completed" ||
    run.conclusion !== "failure" || run.created_at !== "2026-09-29T10:46:12Z" ||
    run.updated_at !== "2026-09-29T10:49:35Z" ||
    run.actor?.login !== "oliverkko" || run.actor?.id !== 309485696 ||
    run.actor?.type !== "User" || !Array.isArray(jobs) || jobs.length !== 5) return false

  const expectedJobs = new Map([
    [109371482728, ["metadata", "failure", "2026-09-29T10:49:21Z", "2026-09-29T10:49:34Z"]],
    [109371565769, ["verify", "skipped", "2026-09-29T10:49:34Z", "2026-09-29T10:49:34Z"]],
    [109371566324, ["binaries", "skipped", "2026-09-29T10:49:34Z", "2026-09-29T10:49:34Z"]],
    [109371566777, ["audit-smoke", "skipped", "2026-09-29T10:49:34Z", "2026-09-29T10:49:34Z"]],
    [109371567499, ["publish", "skipped", "2026-09-29T10:49:34Z", "2026-09-29T10:49:34Z"]],
  ])
  for (const job of jobs) {
    const expected = expectedJobs.get(job?.id)
    if (!expected || job.run_id !== run.id || job.run_attempt !== 2 ||
      job.head_sha !== source || job.status !== "completed" ||
      job.name !== expected[0] || job.conclusion !== expected[1] ||
      job.started_at !== expected[2] || job.completed_at !== expected[3]) return false
  }
  const metadata = jobs.find((job) => job.id === 109371482728)
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
