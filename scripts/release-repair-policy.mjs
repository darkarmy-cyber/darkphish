// One maintainer-authorized repair, not a general release-freeze escape hatch.
const repository = "darkarmy-cyber/darkphish"
const base = "a2851cd96e327175b3d45b8949ad64492b52b21f"
const source = "73bf5948ed19cd918638453d478ef3f1cbe83d89"
const allowed = new Set([
  "scripts/release-maintainer-review.mjs", "scripts/release-maintainer-review-legacy.test.mjs",
  "scripts/release-lib.mjs", "scripts/release-repair-policy.mjs", "scripts/release-repair-policy.test.mjs",
  "changes/historical-release-review-provenance.md", "docs/RELEASE_REPAIR_56.md",
  "scripts/review-gate.mjs", "scripts/review-gate.test.mjs",
])
const resumeAllowed = new Set([
  ".github/workflows/release.yml", ".github/workflows/release-recover.yml",
  ".github/release-normalization-hold.json", ".github/workflows/release-reconcile.yml", ".github/workflows/release-resume.yml",
  "scripts/release-normalization-hold.test.mjs", "scripts/release-reconcile.mjs",
  "scripts/release-resume.mjs", "scripts/release-resume.test.mjs",
  "scripts/release-repair-policy.mjs", "scripts/release-repair-policy.test.mjs",
  "changes/resume-verified-release.md", "docs/RELEASE_RESUME_071.md",
])
const timestampAllowed = new Set([
  "scripts/release-resume.mjs", "scripts/release-resume.test.mjs",
  "scripts/release-repair-policy.mjs", "scripts/release-repair-policy.test.mjs",
  "changes/resumption-publication-timestamp.md", "docs/RELEASE_RESUME_TIMESTAMP.md",
])
const release0201Allowed = new Set([
  ".github/scripts/release-recover-0201-policy.mjs", ".github/scripts/release-recover-0201-policy.test.mjs", ".github/scripts/release-recover.mjs",
  "scripts/changelog.mjs", "scripts/changelog.test.mjs", "scripts/changelog-repair-policy.mjs", "scripts/changelog-repair-policy.test.mjs",
  "scripts/release-maintainer-review.mjs", "scripts/release-maintainer-review-0201.test.mjs",
  "scripts/release-repair-policy.mjs", "scripts/release-repair-policy-0201.test.mjs",
  "changes/recover-v0-20-1-review-provenance.md", "docs/RELEASE_REPAIR_0201.md",
])
const release0201Added = new Set([
  ".github/scripts/release-recover-0201-policy.mjs", ".github/scripts/release-recover-0201-policy.test.mjs",
  "scripts/changelog-repair-policy.mjs", "scripts/changelog-repair-policy.test.mjs",
  "scripts/release-maintainer-review-0201.test.mjs", "scripts/release-repair-policy-0201.test.mjs",
  "changes/recover-v0-20-1-review-provenance.md", "docs/RELEASE_REPAIR_0201.md",
])
const release0201FollowupAllowed = new Set([
  ".github/scripts/release-recover-0201-policy.mjs", ".github/scripts/release-recover-0201-policy.test.mjs",
  "scripts/changelog-repair-policy.mjs", "scripts/changelog-repair-policy.test.mjs",
  "scripts/release-repair-policy.mjs", "scripts/release-repair-policy-0201.test.mjs",
  "changes/recover-v0-20-1-run-manifest.md", "docs/RELEASE_REPAIR_0201.md",
])
const release0202Allowed = new Set([
  ".github/scripts/release-recover-0202-policy.mjs", ".github/scripts/release-recover-0202-policy.test.mjs", ".github/scripts/release-recover.mjs",
  "scripts/changelog-repair-policy.mjs", "scripts/changelog-repair-policy.test.mjs",
  "scripts/release-maintainer-review.mjs", "scripts/release-maintainer-review-0202.test.mjs",
  "scripts/release-repair-policy.mjs", "scripts/release-repair-policy-0202.test.mjs",
  "changes/recover-v0-20-2-review-provenance.md", "docs/RELEASE_REPAIR_0202.md",
])
const release0202Added = new Set([
  ".github/scripts/release-recover-0202-policy.mjs", ".github/scripts/release-recover-0202-policy.test.mjs",
  "scripts/release-maintainer-review-0202.test.mjs", "scripts/release-repair-policy-0202.test.mjs",
  "changes/recover-v0-20-2-review-provenance.md", "docs/RELEASE_REPAIR_0202.md",
])

const release0211Allowed = new Set([
  ".github/scripts/release-recover-0211-policy.mjs", ".github/scripts/release-recover-0211-policy.test.mjs", ".github/scripts/release-recover.mjs",
  "scripts/changelog-repair-policy.mjs", "scripts/changelog-repair-policy.test.mjs",
  "scripts/release-maintainer-review.mjs", "scripts/release-maintainer-review-0211.test.mjs",
  "scripts/release-repair-policy.mjs", "scripts/release-repair-policy-0211.test.mjs",
  "changes/recover-v0-21-1-review-provenance.md",
])
const release0211Added = new Set([
  ".github/scripts/release-recover-0211-policy.mjs", ".github/scripts/release-recover-0211-policy.test.mjs",
  "scripts/release-maintainer-review-0211.test.mjs", "scripts/release-repair-policy-0211.test.mjs",
  "changes/recover-v0-21-1-review-provenance.md",
])

const release0211FollowupAllowed = new Set([
  ".github/scripts/release-recover.mjs", ".github/scripts/release-recover.test.mjs",
  "changes/harden-v0-21-1-recovery.md",
  "scripts/changelog-repair-policy.mjs", "scripts/changelog-repair-policy.test.mjs",
  "scripts/release-repair-policy.mjs", "scripts/release-repair-policy-0211.test.mjs",
])
const release0211FollowupAdded = new Set(["changes/harden-v0-21-1-recovery.md"])

const release0251RepairAllowed = new Set([
  "changes/braces-3-0-4-audit.md",
  "docs/RELEASE_REPAIR_0251.md",
  "pnpm-workspace.yaml",
  "scripts/changelog-repair-policy.mjs",
  "scripts/changelog-repair-policy.test.mjs",
  "scripts/release-pending.mjs",
  "scripts/release-pending.test.mjs",
  "scripts/release-repair-policy.mjs",
  "scripts/release-repair-policy.test.mjs",
])
const release0251RepairAdded = new Set([
  "changes/braces-3-0-4-audit.md",
  "docs/RELEASE_REPAIR_0251.md",
  "pnpm-workspace.yaml",
])

async function hasReleaseWithTag(repo, tag, request) {
  for (let page = 1; page <= 1000; page++) {
    const releases = await request(`repos/${repo}/releases?per_page=100&page=${page}`)
    if (!Array.isArray(releases)) throw new Error("Release inventory is malformed")
    if (releases.some((release) => release?.tag_name === tag)) return true
    if (releases.length < 100) return false
  }
  throw new Error("Release inventory pagination exceeded its safe bound")
}

export async function verifyReleaseRepair(repo, pr, request) {
  const resume = pr.number === 58, timestamp = pr.number === 59, release0201 = pr.number === 117, release0201Followup = pr.number === 118, release0202 = pr.number === 120, release0211 = pr.number === 129, release0211Followup = pr.number === 135, release0251Repair = pr.number === 203
  const expectedBase = release0251Repair ? "98d96c7cc7eebf6ab09bbeffdf109778dd4f3632" : release0211Followup ? "69355444d01c7ef29709ec398fdbacfc4d9b4753" : release0211 ? "d58b43260b1b94a8a7ad6897636f9d0fb9ee06d4" : release0202 ? "4be0bc0d1a6a2f158da5aaed996dcd87bab3bd80" : release0201Followup ? "3cc011581698b73c76512f155f7fd78399345efd" : release0201 ? "cafdcc08b9d968931cb1446891055e3256d0caec" : timestamp ? "697fe42eb20f3f5197be9545ed5b3bf835b730c0" : resume ? "88377d6951ede7352acb257777250bdfecd2052b" : base
  const paths = release0251Repair ? release0251RepairAllowed : release0211Followup ? release0211FollowupAllowed : release0211 ? release0211Allowed : release0202 ? release0202Allowed : release0201Followup ? release0201FollowupAllowed : release0201 ? release0201Allowed : timestamp ? timestampAllowed : resume ? resumeAllowed : allowed
  const branch = release0251Repair ? "fix/braces-audit-0.25.1" : release0211Followup ? "fix/v0.21.1-recovery-hardening" : release0211 ? "fix/v0.21.1-review-recovery" : release0202 ? "fix/recover-v0.20.2-review-provenance" : release0201Followup ? "fix/recover-v0.20.1-run-manifest" : release0201 ? "fix/recover-v0.20.1-review-provenance" : timestamp ? "codex/threads/019fb3b4-63f2-7180-8a29-babee7e6a51b/release071-timestamp" : resume ? "codex/threads/019fb3b4-63f2-7180-8a29-babee7e6a51b/release071-finalize" : "fix/historical-release-review-provenance"
  if (repo !== repository || ![56, 58, 59, 117, 118, 120, 129, 135, 203].includes(pr.number) || pr.base?.sha !== expectedBase || pr.base.ref !== "main" ||
    pr.head?.repo?.full_name !== repository || pr.head.ref !== branch ||
    !/^[a-f0-9]{40}$/.test(pr.head.sha || "") || pr.state !== "open" || pr.draft !== false ||
    !["OWNER", "MEMBER", "COLLABORATOR"].includes(pr.author_association)) throw new Error("Not the authorized release repair")
  const files = await request(`repos/${repo}/pulls/${pr.number}/files?per_page=100&page=1`)
  if (!Array.isArray(files) || !files.length || files.length > paths.size ||
    new Set(files.map(f => f.filename)).size !== files.length ||
    files.some(f => !paths.has(f.filename) || (!(resume && f.filename === ".github/release-normalization-hold.json" && f.status === "removed") && !["added", "modified"].includes(f.status)) || f.previous_filename)) {
    throw new Error("Release repair includes unauthorized paths or file operations")
  }
  if ((release0201 || release0201Followup || release0202 || release0211 || release0211Followup || release0251Repair) && (files.length !== paths.size || files.some(file =>
    file.status !== ((release0251Repair ? release0251RepairAdded : release0211Followup ? release0211FollowupAdded : release0211 ? release0211Added : release0202 ? release0202Added : release0201 ? release0201Added : new Set(["changes/recover-v0-20-1-run-manifest.md"])).has(file.filename) ? "added" : "modified")))) {
    throw new Error("Release recovery repair does not match its exact reviewed file set")
  }
  const read = async (path, ref = pr.head.sha) => {
    const file = await request(`repos/${repo}/contents/${path}?ref=${ref}`)
    if (file?.type !== "file" || file.encoding !== "base64" || typeof file.content !== "string") throw new Error("Missing release repair boundary")
    return Buffer.from(file.content, "base64").toString("utf8")
  }
  if ((await read("VERSION")).trim() !== (release0251Repair ? "0.25.1" : release0211 || release0211Followup ? "0.21.1" : release0202 ? "0.20.2" : release0201 || release0201Followup ? "0.20.1" : "0.7.1")) throw new Error("Release repair VERSION changed")
  if (release0251Repair) {
    const tag = await request(`repos/${repo}/git/ref/tags/v0.25.1`, { missing: true })
    const release = await request(`repos/${repo}/releases/tags/v0.25.1`, { missing: true })
    const stranded = await request(`repos/${repo}/pulls/196`, { missing: true })
    if (tag !== null || release !== null || stranded?.state !== "closed" || stranded?.merged_at !== "2026-10-02T21:03:30Z" ||
      stranded?.merge_commit_sha !== "98d96c7cc7eebf6ab09bbeffdf109778dd4f3632") {
      throw new Error("v0.25.1 repair requires the pinned stranded release and absent tag/release state")
    }
    return
  }
  if (release0211Followup) {
    const tag = await request(`repos/${repo}/git/ref/tags/v0.21.1`, { missing: true })
    if (tag?.object?.type !== "commit" || tag.object.sha !== "51f3c3b56df05d8a6fd165d227cb78d9e46ae821" ||
      await hasReleaseWithTag(repo, "v0.21.1", request)) {
      throw new Error("v0.21.1 hardening requires the pinned tag and absent release state")
    }
    return
  }
  if (release0211) {
    const tag = await request(`repos/${repo}/git/ref/tags/v0.21.1`, { missing: true })
    if (tag !== null || await hasReleaseWithTag(repo, "v0.21.1", request)) throw new Error("v0.21.1 recovery repair requires the reviewed absent tag and release state")
    return
  }
  if (release0202) {
    const tag = await request(`repos/${repo}/git/ref/tags/v0.20.2`, { missing: true })
    const release = await request(`repos/${repo}/releases/tags/v0.20.2`, { missing: true })
    if (tag !== null || release !== null) throw new Error("v0.20.2 recovery repair requires the reviewed absent tag and release state")
    return
  }
  if (release0201 || release0201Followup) {
    const tag = await request(`repos/${repo}/git/ref/tags/v0.20.1`, { missing: true })
    const release = await request(`repos/${repo}/releases/tags/v0.20.1`, { missing: true })
    if (tag !== null || release !== null) throw new Error("v0.20.1 recovery repair requires the reviewed absent tag and release state")
    return
  }
  if (timestamp) {
    for (const ref of [expectedBase, pr.head.sha]) {
      if (await request(`repos/${repo}/contents/.github/release-normalization-hold.json?ref=${ref}`, { missing: true }) !== null) {
        throw new Error("Timestamp repair must preserve the reviewed absence of the temporary hold")
      }
    }
    return
  }
  if (resume) {
    if (!files.some(f => f.filename === ".github/release-normalization-hold.json" && f.status === "removed") ||
      await request(`repos/${repo}/contents/.github/release-normalization-hold.json?ref=${pr.head.sha}`, { missing: true }) !== null) {
      throw new Error("Final resumption must remove exactly the temporary hold")
    }
  }
  const hold = JSON.parse(await read(".github/release-normalization-hold.json", resume ? expectedBase : pr.head.sha))
  if (Object.keys(hold).sort().join(",") !== "schema,source_sha,version" ||
    hold.schema !== "darkphish-release-normalization-hold/v1" || hold.version !== "0.7.1" || hold.source_sha !== source) {
    throw new Error("Release repair must preserve the exact publication hold")
  }
}
