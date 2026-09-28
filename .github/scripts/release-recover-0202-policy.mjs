const repository = "darkarmy-cyber/darkphish"
const version = "0.20.2"
const tag = "v0.20.2"
const source = "4be0bc0d1a6a2f158da5aaed996dcd87bab3bd80"

export function auditedTagless0202Source(candidate) {
  const pr = candidate?.pr
  return candidate?.repository === repository && candidate.version === version && candidate.tag === tag &&
    pr?.number === 119 && pr.state === "closed" && pr.draft === false &&
    pr.title === "release: Darkphish 0.20.2" && pr.merged_at === "2026-09-27T21:50:12Z" &&
    pr.merge_commit_sha === source && pr.base?.ref === "main" &&
    pr.base?.sha === "eef3789a3f7f23b03fd233716a7e0c5209a8c4c9" &&
    pr.head?.ref === "release/v0.20.2" && pr.head?.sha === "64241df5e4225693105e1a5989c12d453f76f8ba" &&
    pr.head?.repo?.full_name === repository ? source : null
}

export function audited0202MetadataFailure(candidate) {
  const { run, jobs } = candidate || {}
  if (candidate?.repository !== repository || candidate.source !== source ||
    run?.id !== 36353391121 || run.run_number !== 352 || run.run_attempt !== 1 ||
    run.workflow_id !== 351159930 || run.name !== "Native release" ||
    run.path !== ".github/workflows/release.yml" || run.event !== "workflow_run" ||
    run.head_branch !== "main" || run.head_sha !== source || run.status !== "completed" ||
    run.conclusion !== "failure" || run.actor?.login !== "oliverkko" || run.actor?.id !== 309485696 ||
    run.actor?.type !== "User" || !Array.isArray(jobs) || jobs.length !== 5) return false

  const expectedJobs = new Map([
    [108716317734, ["metadata", "failure"]],
    [108716363982, ["verify", "skipped"]],
    [108716364413, ["binaries", "skipped"]],
    [108716364445, ["audit-smoke", "skipped"]],
    [108716364937, ["publish", "skipped"]],
  ])
  for (const job of jobs) {
    const expected = expectedJobs.get(job?.id)
    if (!expected || job.run_id !== run.id || job.status !== "completed" ||
      job.name !== expected[0] || job.conclusion !== expected[1]) return false
  }
  const metadata = jobs.find((job) => job.id === 108716317734)
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
    return step?.number === number && step.name === name && step.status === "completed" && step.conclusion === conclusion
  })
}
