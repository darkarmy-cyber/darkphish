// One maintainer-authorized repair, not a general release-freeze escape hatch.
const repository = "darkarmy-cyber/darkphish"
const base = "a2851cd96e327175b3d45b8949ad64492b52b21f"
const source = "73bf5948ed19cd918638453d478ef3f1cbe83d89"
const allowed = new Set([
  "scripts/release-maintainer-review.mjs", "scripts/release-maintainer-review-legacy.test.mjs",
  "scripts/release-lib.mjs", "scripts/release-repair-policy.mjs", "scripts/release-repair-policy.test.mjs",
  "changes/historical-release-review-provenance.md", "docs/RELEASE_REPAIR_56.md",
])

export async function verifyReleaseRepair(repo, pr, request) {
  if (repo !== repository || pr.number !== 56 || pr.base?.sha !== base || pr.base.ref !== "main" ||
    pr.head?.repo?.full_name !== repository || pr.head.ref !== "fix/historical-release-review-provenance" ||
    !/^[a-f0-9]{40}$/.test(pr.head.sha || "") || pr.state !== "open" || pr.draft !== false ||
    !["OWNER", "MEMBER", "COLLABORATOR"].includes(pr.author_association)) throw new Error("Not the authorized release repair")
  const files = await request(`repos/${repo}/pulls/56/files?per_page=100&page=1`)
  if (!Array.isArray(files) || !files.length || files.length > allowed.size ||
    new Set(files.map(f => f.filename)).size !== files.length ||
    files.some(f => !allowed.has(f.filename) || !["added", "modified"].includes(f.status) || f.previous_filename)) {
    throw new Error("Release repair includes unauthorized paths or file operations")
  }
  const read = async path => {
    const file = await request(`repos/${repo}/contents/${path}?ref=${pr.head.sha}`)
    if (file?.type !== "file" || file.encoding !== "base64" || typeof file.content !== "string") throw new Error("Missing release repair boundary")
    return Buffer.from(file.content, "base64").toString("utf8")
  }
  if ((await read("VERSION")).trim() !== "0.7.1") throw new Error("Release repair VERSION changed")
  const hold = JSON.parse(await read(".github/release-normalization-hold.json"))
  if (Object.keys(hold).sort().join(",") !== "schema,source_sha,version" ||
    hold.schema !== "darkphish-release-normalization-hold/v1" || hold.version !== "0.7.1" || hold.source_sha !== source) {
    throw new Error("Release repair must preserve the exact publication hold")
  }
}
