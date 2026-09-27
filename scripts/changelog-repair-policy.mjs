const repository = "darkarmy-cyber/darkphish"
const base = "cafdcc08b9d968931cb1446891055e3256d0caec"
const branch = "fix/recover-v0.20.1-review-provenance"
const files = [
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

export function auditedPendingPatchRepair(candidate) {
  const pull = candidate?.event?.pull_request
  if (candidate?.repository !== repository || candidate.current !== "0.20.1" ||
    candidate.target !== "0.20.2" || candidate.baseSHA !== base ||
    !Array.isArray(candidate.files) || candidate.files.length !== files.length) return false
  const actual = [...candidate.files].sort()
  if (!actual.every((path, index) => path === files[index])) return false
  if (candidate.eventName === "pull_request") {
    return pull?.number === 117 && pull.state === "open" && pull.draft === false &&
      pull.base?.ref === "main" && pull.base?.sha === base &&
      pull.head?.ref === branch && pull.head?.repo?.full_name === repository
  }
  const event = candidate.event
  return candidate.eventName === "push" && event?.ref === "refs/heads/main" &&
    event.before === base && event.after === candidate.headSHA && event.deleted === false &&
    event.forced === false && event.repository?.full_name === repository &&
    Array.isArray(candidate.parentSHAs) && candidate.parentSHAs.length === 1 && candidate.parentSHAs[0] === base &&
    candidate.commitTitle === "fix(release): recover v0.20.1 review provenance (#117)"
}
