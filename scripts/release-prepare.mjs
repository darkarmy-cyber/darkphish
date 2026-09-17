import { execFileSync } from "node:child_process"
import { copyFileSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { api, assertGeneratedCommits, mergeReviewedPullRequest, git, greenCommit, pages, protectedMain, repository, versionTag } from "./release-lib.mjs"
import { assertReleaseAdvancePublished } from "./release-pending.mjs"
import { dispatchChecks } from "./check-refresh.mjs"
import { verifyCodeQLBaseline } from "./codeql-baseline.mjs"

function trustedGeneratedReleasePR(repo, branch, version, pr) {
  if (!pr || pr.state !== "open" || pr.draft !== false || pr.base?.ref !== "main" ||
    pr.head?.ref !== branch || pr.head?.repo?.full_name !== repo ||
    pr.title !== `release: Darkphish ${version}` ||
    pr.user?.login !== "github-actions[bot]" || pr.user?.id !== 41898282 || pr.user?.type !== "Bot") {
    throw new Error("existing release PR is not the trusted generated release pull request")
  }
  return pr
}

function trustedReleaseDate(pr) {
  const timestamp = pr?.created_at
  const value = typeof timestamp === "string" ? timestamp.slice(0, 10) : ""
  if (typeof timestamp !== "string" || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{3})?Z$/.test(timestamp) ||
    !Number.isFinite(Date.parse(timestamp)) || new Date(timestamp).toISOString().slice(0, 10) !== value) {
    throw new Error("existing release PR is missing a valid server creation date")
  }
  return value
}

function generateReleaseChangelog(root, version, releaseDate) {
  execFileSync(process.execPath, [join(root, "scripts/changelog.mjs"), "prepare"], { cwd: root, stdio: "pipe" })
  const changelogPath = join(root, "CHANGELOG.md")
  const generated = readFileSync(changelogPath, "utf8").replace(/\r\n/g, "\n")
  const heading = `## ${version} - `
  const lines = generated.split("\n")
  const headings = lines.flatMap((line, index) => line.startsWith(heading) ? [index] : [])
  if (headings.length !== 1) throw new Error("trusted generated release output is missing a unique expected changelog heading")
  lines[headings[0]] = `${heading}${releaseDate}`
  writeFileSync(changelogPath, lines.join("\n"))
}

async function prepare() {
  const repo = repository()
  const sha = git("rev-parse", "HEAD")
  const version = execFileSync(process.execPath, ["scripts/changelog.mjs", "target"], { encoding: "utf8" }).trim()
  const tag = versionTag(version)
  const branch = `release/${tag}`
  await protectedMain(repo, sha)
  const currentVersion = readFileSync("VERSION", "utf8").trim()
  await assertReleaseAdvancePublished(repo, currentVersion, version)
  if (await api(`repos/${repo}/git/ref/tags/${tag}`, { missing: true }) || await api(`repos/${repo}/releases/tags/${tag}`, { missing: true })) {
    console.log("Release tag or release already exists; preparation leaves it unchanged.")
    return
  }
  if (!await greenCommit(repo, sha)) {
    await dispatchChecks(repo, "main")
    console.log("Waiting for all required main CI and security checks before release preparation.")
    return
  }
  await verifyCodeQLBaseline(repo, sha)
  if (!readdirSync("changes").some((name) => name.endsWith(".md") && name !== "README.md")) {
    console.log("No unconsumed release fragments; protected publication workflow owns the next step.")
    return
  }
  execFileSync(process.execPath, ["scripts/changelog.mjs", "validate"], { stdio: "inherit" })
  const ref = await api(`repos/${repo}/git/ref/heads/${branch}`, { missing: true })
  const prs = await pages(`repos/${repo}/pulls?state=open&base=main&head=${repo.split("/")[0]}:${branch}`)
  if (prs.length > 1) throw new Error("multiple release PRs exist; refusing ambiguous recovery")
  try {
    const permissions = await api(`repos/${repo}/actions/permissions/workflow`)
    if (!permissions.can_approve_pull_request_reviews) throw new Error("Enable Settings > Actions > General > Allow GitHub Actions to create and approve pull requests. Workflows never submit reviews.")
  } catch (error) {
    if (error.status !== 403) throw error
    console.log("GitHub does not expose the PR-creation setting to this token; PR creation will be attempted after all other preflight checks.")
  }
  const oldSHA = ref?.object.sha
  // Capture once for a new PR; existing PRs use only trusted server metadata.
  let releaseDate = new Date().toISOString().slice(0, 10)
  if (oldSHA) {
    git("fetch", "--no-tags", "origin", branch)
    const base = git("merge-base", sha, oldSHA)
    const commits = git("log", "--format=%an%x09%s", `${sha}..${oldSHA}`).split("\n").filter(Boolean).map((line) => { const [author, subject] = line.split("\t"); return { author, subject } })
    assertGeneratedCommits(commits, git("diff", "--name-only", `${base}..${oldSHA}`).split("\n").filter(Boolean), version)
    const existingPR = trustedGeneratedReleasePR(repo, branch, version, prs[0])
    releaseDate = trustedReleaseDate(existingPR)
    const temporary = mkdtempSync(join(tmpdir(), "darkphish-release-check-"))
    try {
      git("worktree", "add", "--detach", temporary, base)
      copyFileSync("scripts/changelog.mjs", join(temporary, "scripts/changelog.mjs"))
      generateReleaseChangelog(temporary, version, releaseDate)
      const expected = execFileSync("git", ["diff", "--", "VERSION", "CHANGELOG.md", "changes"], { cwd: temporary, encoding: "utf8" }).replace(/\r\n/g, "\n")
      const actual = execFileSync("git", ["diff", base, oldSHA, "--", "VERSION", "CHANGELOG.md", "changes"], { encoding: "utf8" }).replace(/\r\n/g, "\n")
      if (expected !== actual) throw new Error("release branch differs from trusted generated output; refusing to overwrite it")
    } finally {
      try { git("worktree", "remove", "--force", temporary) } finally { rmSync(temporary, { recursive: true, force: true }) }
    }
  }
  generateReleaseChangelog(".", version, releaseDate)
  git("add", "--", "VERSION", "CHANGELOG.md", "changes")
  const tree = git("write-tree")
  let releaseSHA = oldSHA
  if (!oldSHA || tree !== git("rev-parse", `${oldSHA}^{tree}`)) {
    git("config", "user.name", "github-actions[bot]")
    git("config", "user.email", "41898282+github-actions[bot]@users.noreply.github.com")
    const parents = oldSHA && oldSHA !== sha ? ["-p", oldSHA, "-p", sha] : ["-p", sha]
    releaseSHA = git("commit-tree", tree, ...parents, "-m", `release: Darkphish ${version}`)
    git("push", "origin", `${releaseSHA}:refs/heads/${branch}`)
  }
  let pr = prs[0]
  let created = false
  if (!pr) {
    try {
      pr = await api(`repos/${repo}/pulls`, { method: "POST", body: { base: "main", head: branch, title: `release: Darkphish ${version}`, body: `Prepare Darkphish ${version} from protected main after all required checks.\n\nThis PR contains only generated changelog aggregation and consumed-fragment removal. Native publication starts only after this PR passes the same required checks and merges through repository protection.` } })
      created = true
    } catch (error) {
      throw new Error(`${error.message}. Enable Settings > Actions > General > Workflow permissions > Allow GitHub Actions to create and approve pull requests. The generated branch is safe to reuse; no review approval is fabricated.`)
    }
  }
  if (created) await api(`repos/${repo}/issues/${pr.number}/labels`, { method: "POST", body: { labels: ["codex-automerge"] } })
  const refreshed = await dispatchChecks(repo, branch)
  if (refreshed.dispatched || !await greenCommit(repo, refreshed.sha)) {
    console.log(`Release PR #${pr.number}: exact-head CI/CodeQL refresh requested for ${refreshed.sha}; merge waits for successful required checks.`)
    return
  }
  await mergeReviewedPullRequest(repo, await api(`repos/${repo}/pulls/${pr.number}`), { releaseMerge: true })
  console.log(`Prepared ${pr.html_url} at ${releaseSHA}; recovery requires exact-head code and explicit security review before protected merge.`)
}

prepare().catch((error) => { console.error(error.message); process.exitCode = 1 })
