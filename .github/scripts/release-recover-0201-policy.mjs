const repository = "darkarmy-cyber/darkphish"
const version = "0.20.1"
const tag = "v0.20.1"
const source = "cafdcc08b9d968931cb1446891055e3256d0caec"

export function auditedTagless0201Source(candidate) {
  const pr = candidate?.pr
  return candidate?.repository === repository && candidate.version === version && candidate.tag === tag &&
    pr?.number === 116 && pr.state === "closed" && pr.draft === false &&
    pr.title === "release: Darkphish 0.20.1" && pr.merged_at === "2026-09-27T19:02:26Z" &&
    pr.merge_commit_sha === source && pr.base?.ref === "main" &&
    pr.base?.sha === "1ad4467eaca1f6d182e4f0458f39cda58e5d3ce6" &&
    pr.head?.ref === "release/v0.20.1" && pr.head?.sha === "5bd47cde0983e0506fd1dc177efe59cfd1960ef9" &&
    pr.head?.repo?.full_name === repository ? source : null
}

export function audited0201MetadataFailure(candidate) {
  const { run, jobs } = candidate || {}
  if (candidate?.repository !== repository || candidate.source !== source ||
    run?.id !== 36343131819 || run.run_number !== 345 || run.run_attempt !== 1 ||
    run.workflow_id !== 351159930 || run.name !== "Native release" ||
    run.path !== ".github/workflows/release.yml" || run.event !== "workflow_run" ||
    run.head_branch !== "main" || run.head_sha !== source || run.status !== "completed" ||
    run.conclusion !== "failure" || run.actor?.login !== "oliverkko" || run.actor?.id !== 309485696 ||
    run.actor?.type !== "User" || !Array.isArray(jobs) || jobs.length !== 5) return false

  const expectedJobs = new Map([
    [108687061302, ["metadata", "failure"]],
    [108687103646, ["verify", "skipped"]],
    [108687104304, ["binaries", "skipped"]],
    [108687104319, ["audit-smoke", "skipped"]],
    [108687104828, ["publish", "skipped"]],
  ])
  for (const job of jobs) {
    const expected = expectedJobs.get(job?.id)
    if (!expected || job.run_id !== run.id || job.status !== "completed" ||
      job.name !== expected[0] || job.conclusion !== expected[1]) return false
  }
  const metadata = jobs.find((job) => job.id === 108687061302)
  const steps = metadata?.steps
  if (!Array.isArray(steps) || steps.length !== 8) return false
  const expectedSteps = [
    [1, "Set up job", "success"],
    [2, "Run actions/checkout@v7", "success"],
    [3, "Run actions/setup-node@v7", "success"],
    [4, "Run node scripts/changelog.mjs validate", "success"],
    [5, "Enforce release normalization hold", "success"],
    [6, "Run node scripts/release-publish.mjs metadata", "failure"],
    [11, "Post Run actions/setup-node@v7", "skipped"],
    [12, "Post Run actions/checkout@v7", "success"],
  ]
  return expectedSteps.every(([number, name, conclusion], index) => {
    const step = steps[index]
    return step?.number === number && step.name === name && step.status === "completed" && step.conclusion === conclusion
  })
}
