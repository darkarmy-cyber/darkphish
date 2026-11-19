// One-shot authorization for the stranded v0.27.2 release; never a generic freeze bypass.
const repoName = "darkarmy-cyber/darkphish"
const baseSHA = "3f103ded1bcae7e8eccaaf46c81b910c591586cd"
const originalHead = "cc600c1cfb741a30cf3b4d85917c1d57545ae49c"
const originalMerge = baseSHA
const paths = new Set([
  "docs/RECOVERY_V0_27_2.md",
  "changes/recover-v0-27-2-attestation.md",
  "scripts/release-repair-policy.mjs",
  "scripts/release-repair-0272.mjs",
  "scripts/release-repair-0272.test.mjs",
])

export async function verifyV0272Repair(repo, pr, request) {
  if (repo !== repoName || pr?.number !== 247 || pr.state !== "open" || pr.draft !== false ||
      pr.title !== "fix(release): recover v0.27.2 through fresh attested release PR" ||
      pr.base?.ref !== "main" || pr.base?.sha !== baseSHA ||
      pr.head?.repo?.full_name !== repo || pr.head.ref !== "fix/v0-27-2-release-provenance-recovery" ||
      !/^[a-f0-9]{40}$/.test(pr.head.sha || "") ||
      !["OWNER", "MEMBER", "COLLABORATOR"].includes(pr.author_association)) {
    throw new Error("Not the exact authorized v0.27.2 release repair PR")
  }
  const files = await request(`repos/${repo}/pulls/247/files?per_page=100&page=1`)
  if (!Array.isArray(files) || files.length !== paths.size ||
      new Set(files.map(file => file.filename)).size !== paths.size ||
      files.some(file => !paths.has(file.filename) ||
        file.status !== (["docs/RECOVERY_V0_27_2.md", "changes/recover-v0-27-2-attestation.md",
          "scripts/release-repair-0272.mjs", "scripts/release-repair-0272.test.mjs"].includes(file.filename)
          ? "added" : "modified") || file.previous_filename)) {
    throw new Error("v0.27.2 release repair changed unexpected paths")
  }
  const old = await request(`repos/${repo}/pulls/244`)
  if (old?.state !== "closed" || old.merged_at !== "2026-11-19T13:55:57Z" ||
      old.head?.sha !== originalHead || old.merge_commit_sha !== originalMerge ||
      old.base?.ref !== "main" || old.head?.ref !== "release/v0.27.2") {
    throw new Error("Original v0.27.2 merge evidence changed")
  }
  if (await request(`repos/${repo}/git/ref/tags/v0.27.2`, { missing: true }) !== null ||
      await request(`repos/${repo}/releases/tags/v0.27.2`, { missing: true }) !== null) {
    throw new Error("v0.27.2 tag or release already exists; refuse recovery")
  }
  const read = async path => {
    const item = await request(`repos/${repo}/contents/${path}?ref=${pr.head.sha}`)
    if (item?.type !== "file" || item.encoding !== "base64" || typeof item.content !== "string") {
      throw new Error("Missing v0.27.2 repair source")
    }
    return Buffer.from(item.content, "base64").toString("utf8")
  }
  if ((await read("VERSION")).trim() !== "0.27.2") throw new Error("Repair changed current VERSION")
  const expectedFragment = "---\ncategory: Security\nversion: 0.27.2\n---\n- Restore the v0.27.2 release publication path by requiring a new generated release pull request with complete pre-merge security and maintainer review attestations; the original unreviewed release merge remains unpublished.\n"
  if (await read("changes/recover-v0-27-2-attestation.md") !== expectedFragment) {
    throw new Error("v0.27.2 recovery fragment differs from authorized scope")
  }
  // A successful result only authorizes the reviewed repair PR, not release
  // publication. Fresh generated PR review, CI, CodeQL, artifacts and receipt
  // remain mandatory for the next protected publication.
}
