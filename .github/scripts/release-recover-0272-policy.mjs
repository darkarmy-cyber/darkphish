// Audited v0.27.2 incident: the successful native run was re-run from an
// earlier event SHA while its metadata selected the newer release source.
// This run is evidence of an attempted publication only. It NEVER qualifies
// as canonical native provenance; recovery must rebuild and attest all bytes.
const repo = "darkarmy-cyber/darkphish"
const source = "7bfe25c0f122f98196978bdd0ad9b679b65cc8fd"
const staleHead = "c77b486b461c817c8b936edfeb03db1cbce593e3"
const runId = 44900262093
const releaseId = 453734839
const bot = actor => actor?.login === "github-actions[bot]" && actor.type === "Bot" && actor.id === 41898282
const original = new Map([
  ["darkphish-v0.27.2-darwin-amd64.tar.gz", [775775206, 31306185, "sha256:786b350cced810d8c5a1664fe66acc03b39155295598fb46f23a960c65ee4719"]],
  ["darkphish-v0.27.2-darwin-arm64.tar.gz", [775775272, 30609544, "sha256:bd96b93c161600b66b358b6adf075ba58483b146ecab16d6a758adc0a1dd8dc5"]],
  ["darkphish-v0.27.2-linux-amd64.tar.gz", [775775360, 31586684, "sha256:8ad4ad869ad8f161b6d58fa2dd4575cead2b0a282dbbce0b41b97ac9df089f7d"]],
  ["darkphish-v0.27.2-linux-arm64.tar.gz", [775775458, 30752040, "sha256:6ea558691f1d5c2d16407abf9596de68371f23ea21ebeafa1d55b1c41e891ae6"]],
  ["darkphish-v0.27.2-windows-amd64.zip", [775775538, 32394061, "sha256:2080affe3e658cb4f080a6c953fb78b63b4c8c399357ddeb4d2fdc5b8d064073"]],
  ["darkphish-v0.27.2.release.json", [775776020, 212, "sha256:534993b3ba8858d825d048f210ee55b99002f27feb5fd7c3433268d58f5a954b"]],
  ["darkphish-v0.27.2.spdx.json", [775775613, 708614, "sha256:d617825e29f2126feff395ae8938d3318af47a18d54f65db374059954ecbaa5f"]],
  ["SHA256SUMS", [775775175, 610, "sha256:40c3d0340663924ca83025117762685d46b27cc2114b065932c24eebf4fab60b"]],
])

export const audited0272Source = source
export const audited0272RunId = runId

export function audited0272StaleNativeRun(candidate) {
  const { run, jobs } = candidate || {}
  if (candidate?.repository !== repo || candidate.source !== source ||
    run?.id !== runId || run.run_number !== 839 || run.run_attempt !== 2 ||
    run.workflow_id !== 351159930 || run.name !== "Native release" ||
    run.path !== ".github/workflows/release.yml" || run.event !== "schedule" ||
    run.head_branch !== "main" || run.head_sha !== staleHead ||
    run.status !== "completed" || run.conclusion !== "success" ||
    run.created_at !== "2026-11-21T11:41:00Z" || run.updated_at !== "2026-11-21T14:06:16Z" ||
    !bot(run.actor) || run.triggering_actor?.login !== "oliverkko" ||
    run.triggering_actor?.id !== 309485696 || run.triggering_actor?.type !== "User" ||
    !Array.isArray(jobs) || jobs.length !== 9) return false
  const expected = new Map([
    [136868091231, "metadata"], [136868172989, "verify"],
    [136868607609, "binaries (ubuntu-24.04-arm, linux, arm64, darkphish)"],
    [136868607646, "binaries (ubuntu-24.04, linux, amd64, darkphish)"],
    [136868607651, "binaries (macos-15-intel, darwin, amd64, darkphish)"],
    [136868607656, "binaries (windows-2025, windows, amd64, darkphish.exe)"],
    [136868607693, "binaries (macos-15, darwin, arm64, darkphish)"],
    [136869616287, "audit-smoke"], [136869835738, "publish"],
  ])
  if (new Set(jobs.map(job => job?.id)).size !== expected.size ||
    jobs.some(job => job.run_id !== runId || job.run_attempt !== 2 ||
      job.head_sha !== staleHead || job.status !== "completed" ||
      job.conclusion !== "success" || job.name !== expected.get(job.id))) return false
  const publish = jobs.find(job => job.name === "publish")
  return ["Generate checksums", "Generate canonical native publication receipt",
    "Attest canonical native release artifacts", "Publish verified assets without overwriting an existing release"]
    .every(name => publish?.steps?.some(step => step.name === name && step.status === "completed" && step.conclusion === "success"))
}

export function audited0272Draft(release, version, sourceSHA) {
  return version === "0.27.2" && sourceSHA === source &&
    release?.id === releaseId && release.tag_name === "v0.27.2" &&
    release.target_commitish === source && release.draft === true &&
    release.prerelease === false && bot(release.author)
}

export function audited0272QuarantinedAsset(release, version, sourceSHA, asset) {
  if (!audited0272Draft(release, version, sourceSHA) || !bot(asset?.uploader) || asset.state !== "uploaded") return false
  const pinned = original.get(asset.name)
  return Boolean(pinned && asset.id === pinned[0] && asset.size === pinned[1] && asset.digest === pinned[2])
}
