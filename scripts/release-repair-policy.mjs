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

export async function verifyReleaseRepair(repo, pr, request) {
  const resume = pr.number === 58, expectedBase = resume ? "88377d6951ede7352acb257777250bdfecd2052b" : base
  const paths = resume ? resumeAllowed : allowed
  const branch = resume ? "codex/threads/019fb3b4-63f2-7180-8a29-babee7e6a51b/release071-finalize" : "fix/historical-release-review-provenance"
  if (repo !== repository || ![56, 58].includes(pr.number) || pr.base?.sha !== expectedBase || pr.base.ref !== "main" ||
    pr.head?.repo?.full_name !== repository || pr.head.ref !== branch ||
    !/^[a-f0-9]{40}$/.test(pr.head.sha || "") || pr.state !== "open" || pr.draft !== false ||
    !["OWNER", "MEMBER", "COLLABORATOR"].includes(pr.author_association)) throw new Error("Not the authorized release repair")
  const files = await request(`repos/${repo}/pulls/${pr.number}/files?per_page=100&page=1`)
  if (!Array.isArray(files) || !files.length || files.length > paths.size ||
    new Set(files.map(f => f.filename)).size !== files.length ||
    files.some(f => !paths.has(f.filename) || (!(resume && f.filename === ".github/release-normalization-hold.json" && f.status === "removed") && !["added", "modified"].includes(f.status)) || f.previous_filename)) {
    throw new Error("Release repair includes unauthorized paths or file operations")
  }
  const read = async (path, ref = pr.head.sha) => {
    const file = await request(`repos/${repo}/contents/${path}?ref=${ref}`)
    if (file?.type !== "file" || file.encoding !== "base64" || typeof file.content !== "string") throw new Error("Missing release repair boundary")
    return Buffer.from(file.content, "base64").toString("utf8")
  }
  if ((await read("VERSION")).trim() !== "0.7.1") throw new Error("Release repair VERSION changed")
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
