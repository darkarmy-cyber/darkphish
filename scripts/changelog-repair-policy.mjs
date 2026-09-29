const repository = "darkarmy-cyber/darkphish"
const base = "cafdcc08b9d968931cb1446891055e3256d0caec"
const branch = "fix/recover-v0.20.1-review-provenance"
const files117 = [
  ".github/scripts/release-recover-0201-policy.mjs",
  ".github/scripts/release-recover-0201-policy.test.mjs",
  ".github/scripts/release-recover.mjs",
  "changes/recover-v0-20-1-review-provenance.md",
  "docs/RELEASE_REPAIR_0201.md",
  "scripts/changelog-repair-policy.mjs",
  "scripts/changelog-repair-policy.test.mjs",
  "scripts/changelog.mjs",
  "scripts/changelog.test.mjs",
  "scripts/release-maintainer-review-0201.test.mjs",
  "scripts/release-maintainer-review.mjs",
  "scripts/release-repair-policy-0201.test.mjs",
  "scripts/release-repair-policy.mjs",
].sort()
const files118 = [
  ".github/scripts/release-recover-0201-policy.mjs",
  ".github/scripts/release-recover-0201-policy.test.mjs",
  "changes/recover-v0-20-1-run-manifest.md",
  "docs/RELEASE_REPAIR_0201.md",
  "scripts/changelog-repair-policy.mjs",
  "scripts/changelog-repair-policy.test.mjs",
  "scripts/release-repair-policy-0201.test.mjs",
  "scripts/release-repair-policy.mjs",
].sort()
const files120 = [
  ".github/scripts/release-recover-0202-policy.mjs",
  ".github/scripts/release-recover-0202-policy.test.mjs",
  ".github/scripts/release-recover.mjs",
  "changes/recover-v0-20-2-review-provenance.md",
  "docs/RELEASE_REPAIR_0202.md",
  "scripts/changelog-repair-policy.mjs",
  "scripts/changelog-repair-policy.test.mjs",
  "scripts/release-maintainer-review-0202.test.mjs",
  "scripts/release-maintainer-review.mjs",
  "scripts/release-repair-policy-0202.test.mjs",
  "scripts/release-repair-policy.mjs",
].sort()
const files129 = [
  ".github/scripts/release-recover-0211-policy.mjs",
  ".github/scripts/release-recover-0211-policy.test.mjs",
  ".github/scripts/release-recover.mjs",
  "changes/recover-v0-21-1-review-provenance.md",
  "scripts/changelog-repair-policy.mjs",
  "scripts/changelog-repair-policy.test.mjs",
  "scripts/release-maintainer-review-0211.test.mjs",
  "scripts/release-maintainer-review.mjs",
].sort()
const repairs = [
  { number: 117, current: "0.20.1", target: "0.20.2", base, branch, files: files117, title: "fix(release): recover v0.20.1 review provenance (#117)" },
  { number: 118, current: "0.20.1", target: "0.20.2", base: "3cc011581698b73c76512f155f7fd78399345efd", branch: "fix/recover-v0.20.1-run-manifest", files: files118,
    title: "fix(release): correct v0.20.1 run manifest (#118)" },
  { number: 120, current: "0.20.2", target: "0.20.3", base: "4be0bc0d1a6a2f158da5aaed996dcd87bab3bd80", branch: "fix/recover-v0.20.2-review-provenance", files: files120,
    title: "fix(release): recover v0.20.2 review provenance (#120)" },
  { number: 129, current: "0.21.1", target: "0.21.2", base: "51f3c3b56df05d8a6fd165d227cb78d9e46ae821", branch: "fix/v0.21.1-review-recovery", files: files129,
    title: "fix(release): audit v0.21.1 review recovery (#129)" },
]

export function auditedPendingPatchRepair(candidate) {
  const pull = candidate?.event?.pull_request
  if (candidate?.repository !== repository || !Array.isArray(candidate.files)) return false
  const repair = repairs.find((item) => item.base === candidate.baseSHA && item.files.length === candidate.files.length)
  if (!repair || candidate.current !== repair.current || candidate.target !== repair.target) return false
  const actual = [...candidate.files].sort()
  if (!actual.every((path, index) => path === repair.files[index])) return false
  if (candidate.eventName === "pull_request") {
    return pull?.number === repair.number && pull.state === "open" && pull.draft === false &&
      pull.base?.ref === "main" && pull.base?.sha === repair.base &&
      pull.head?.ref === repair.branch && pull.head?.repo?.full_name === repository
  }
  const event = candidate.event
  return candidate.eventName === "push" && event?.ref === "refs/heads/main" &&
    event.before === repair.base && event.after === candidate.headSHA && event.deleted === false &&
    event.forced === false && event.repository?.full_name === repository &&
    Array.isArray(candidate.parentSHAs) && candidate.parentSHAs.length === 1 && candidate.parentSHAs[0] === repair.base &&
    candidate.commitTitle === repair.title
}
