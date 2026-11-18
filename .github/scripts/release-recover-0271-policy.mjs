// One-shot, fail-closed recovery evidence for v0.27.1 only.
// This metadata-only Native release did NOT build or publish artifacts and
// MUST NEVER qualify as canonical native publication. It authorizes only a
// freshly rebuilt, checksum-verified, smoke-tested, attested recovery release.
const repo = "darkarmy-cyber/darkphish"
const source = "28a3de27bef29224a23d818dfe61db9f47d5711e"
const runId = 43911299954
const releaseId = 447354479
const bot = actor => actor?.login === "github-actions[bot]" && actor.type === "Bot" && actor.id === 41898282
const original = new Map([
  ["darkphish-v0.27.1-darwin-amd64.tar.gz", [755730409, 31246891, "sha256:6b615b3ae46a37aea8eda8cce593ff7e2472c842dc881e00dfac64ab801bec3e"]],
  ["darkphish-v0.27.1-darwin-arm64.tar.gz", [755730486, 30559747, "sha256:0ed2397924fbf5e4effecc02602b02078132c475e745ba787efd6bf9f355cb96"]],
  ["darkphish-v0.27.1-linux-amd64.tar.gz", [755730676, 31530387, "sha256:924b08f7edbe1bad497cadafbc9cd78b54439b1c0ba8a6c7d7eca1f8217cec90"]],
  ["darkphish-v0.27.1-linux-arm64.tar.gz", [755730822, 30698780, "sha256:cdf7d4edbc440bb1cf591df69d7eec2623deb0e6f951931620038b9d0dd32d92"]],
  ["darkphish-v0.27.1-windows-amd64.zip", [755730952, 32334952, "sha256:e1ab11e1f3089905dd8bbc368a4310b8d34daced58b053c90106fc40496f2b52"]],
  ["darkphish-v0.27.1.release.json", [755731619, 212, "sha256:3eeeac3739ab4269cfd17b6a9e37ae0f4424a811b814fff68a714c6c2996b3f7"]],
  ["darkphish-v0.27.1.spdx.json", [755731080, 703576, "sha256:f42bd54cf783ffe8be3139be3a75b4f57176822bea893ec429cf2016314cb09b"]],
  ["SHA256SUMS", [755730348, 610, "sha256:92848be31ea07c45d71d40affe628809d14bff33b5783908a178f378fc07a138"]],
])
export function audited0271MetadataOnly(candidate) {
  const { run, jobs } = candidate || {}
  if (candidate?.repository !== repo || candidate.source !== source ||
    run?.id !== runId || run.run_number !== 792 || run.run_attempt !== 1 ||
    run.workflow_id !== 351159930 || run.name !== "Native release" ||
    run.path !== ".github/workflows/release.yml" || run.event !== "workflow_run" ||
    run.head_branch !== "main" || run.head_sha !== source || run.status !== "completed" ||
    run.conclusion !== "success" || run.created_at !== "2026-11-16T16:47:05Z" ||
    run.updated_at !== "2026-11-16T17:24:16Z" || run.actor?.login !== "oliverkko" ||
    run.actor.id !== 309485696 || run.actor.type !== "User" ||
    !Array.isArray(jobs) || jobs.length !== 5) return false
  const expectedJobs = new Map([
    [133477034203, ["metadata", "success", "2026-11-16T17:23:54Z", "2026-11-16T17:24:14Z"]],
    [133477284605, ["verify", "skipped", "2026-11-16T17:24:15Z", "2026-11-16T17:24:15Z"]],
    [133477286539, ["binaries", "skipped", "2026-11-16T17:24:15Z", "2026-11-16T17:24:15Z"]],
    [133477287908, ["audit-smoke", "skipped", "2026-11-16T17:24:15Z", "2026-11-16T17:24:15Z"]],
    [133477288121, ["publish", "skipped", "2026-11-16T17:24:15Z", "2026-11-16T17:24:15Z"]],
  ])
  if (new Set(jobs.map(j => j?.id)).size !== 5) return false
  for (const job of jobs) {
    const e = expectedJobs.get(job?.id)
    if (!e || job.run_id !== runId || job.run_attempt !== 1 || job.head_sha !== source ||
      job.status !== "completed" || job.name !== e[0] || job.conclusion !== e[1] ||
      job.started_at !== e[2] || job.completed_at !== e[3]) return false
  }
  const metadata = jobs[0]
  if (metadata.id !== 133477034203 || !Array.isArray(metadata.steps) || metadata.steps.length !== 9) return false
  const steps = [
    [1, "Set up job", "success"],
    [2, "Run actions/checkout@v7", "success"],
    [3, "Run actions/setup-node@v7", "success"],
    [4, "Run node scripts/changelog.mjs validate", "success"],
    [5, "Enforce release normalization hold", "success"],
    [6, "Run node scripts/release-publish.mjs metadata", "success"],
    [11, "Post Run actions/setup-node@v7", "success"],
    [12, "Post Run actions/checkout@v7", "success"],
    [13, "Complete job", "success"],
  ]
  return steps.every(([number, name, conclusion], i) => {
    const s = metadata.steps[i]
    return s?.number === number && s.name === name &&
      s.status === "completed" && s.conclusion === conclusion
  })
}
export function audited0271Draft(release, version, sourceSHA) {
  return version === "0.27.1" && sourceSHA === source &&
    release?.id === releaseId && release.tag_name === "v0.27.1" &&
    release.target_commitish === source && release.draft === true &&
    release.prerelease === false && bot(release.author)
}
// The quarantined originals may only be deleted by exact ID, size, digest,
// name and immutable bot identity, never by untrusted name alone.
export function audited0271QuarantinedAsset(release, version, sourceSHA, asset) {
  if (!audited0271Draft(release, version, sourceSHA) || !bot(asset?.uploader) ||
    asset.state !== "uploaded") return false
  const pinned = original.get(asset.name)
  return Boolean(pinned && asset.id === pinned[0] && asset.size === pinned[1] && asset.digest === pinned[2])
}
