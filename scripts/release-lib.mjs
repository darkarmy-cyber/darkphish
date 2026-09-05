import { execFileSync } from "node:child_process"
import { readFileSync } from "node:fs"

export const requiredChecks = JSON.parse(readFileSync(new URL("../.github/required-checks.json", import.meta.url), "utf8"))
export const generatedPath = (path) => path === "VERSION" || path === "CHANGELOG.md" || /^changes\/[a-z0-9][a-z0-9-]*\.md$/.test(path)
export const versionTag = (value) => {
  if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(value)) throw new Error("release VERSION must be stable SemVer")
  return `v${value}`
}
export function checksPassed(checks, names = requiredChecks) {
  return names.every((name) => {
    const latest = checks.filter((check) => check.name === name && check.app?.slug === "github-actions")
      .sort((a, b) => b.id - a.id)[0]
    return latest?.status === "completed" && latest.conclusion === "success"
  })
}
export function assertGeneratedCommits(commits, files, version) {
  if (!commits.length || commits.some((commit) => commit.author !== "github-actions[bot]" || commit.subject !== `release: Darkphish ${version}`)) {
    throw new Error("release branch contains unknown commits; refusing to overwrite developer work")
  }
  if (!files.length || files.some((file) => !generatedPath(file))) throw new Error("release branch contains non-generated changes")
}
export function assertReleaseState(tagSHA, release, sha) {
  if (!tagSHA && !release) return "new"
  if (!tagSHA && release?.draft && release.target_commitish === sha && release.body?.includes(`<!-- darkphish-release-source:${sha} -->`)) return "resume"
  if (tagSHA !== sha) throw new Error("release tag already exists at an unexpected commit; tags are immutable")
  if (!release || !release.body?.includes(`<!-- darkphish-release-source:${sha} -->`)) throw new Error("existing release state is not a known automation artifact")
  return release.draft ? "resume" : "published"
}
export function assetDisposition(existing, digest, size) {
  if (!existing) return "upload"
  if (existing.digest !== `sha256:${digest}` || existing.size !== size) throw new Error("existing release asset differs; refusing to replace a released binary")
  return "reuse"
}
export function protectedMergeArguments(repo, pr) {
  if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repo) || !Number.isSafeInteger(pr.number) || pr.number < 1 || !/^[a-f0-9]{40}$/.test(pr.head?.sha || "")) throw new Error("invalid protected merge target")
  if (pr.draft || pr.base?.ref !== "main" || pr.head.repo?.full_name !== repo) throw new Error("only internal ready main pull requests are eligible")
  return ["pr", "merge", String(pr.number), "--repo", repo, "--auto", "--squash", "--match-head-commit", pr.head.sha]
}
export function verifyChecksums(manifest, hashes) {
  const seen = new Set()
  const lines = manifest.trim().split(/\r?\n/)
  for (const line of lines) {
    const match = line.match(/^([a-f0-9]{64})  (darkphish-[A-Za-z0-9._-]+)$/)
    if (!match || seen.has(match[2]) || hashes.get(match[2]) !== match[1]) throw new Error("release checksum manifest does not match the native artifacts")
    seen.add(match[2])
  }
  if (seen.size !== hashes.size - 1 || [...hashes.keys()].some((name) => name !== "SHA256SUMS" && !seen.has(name))) throw new Error("release checksum manifest is incomplete")
}
export function git(...args) {
  return execFileSync("git", args, { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim()
}
export function repository() {
  const repo = process.env.GITHUB_REPOSITORY || process.env.GH_REPO
  if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repo || "")) throw new Error("GITHUB_REPOSITORY is required")
  return repo
}
export async function api(path, { method = "GET", body, missing = false } = {}) {
  const response = await fetch(`https://api.github.com/${path}`, {
    method,
    headers: { Accept: "application/vnd.github+json", Authorization: `Bearer ${process.env.GH_TOKEN || process.env.GITHUB_TOKEN}`, "X-GitHub-Api-Version": "2022-11-28" },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (response.status === 404 && missing) return null
  if (!response.ok) {
    const error = new Error(`GitHub ${method} ${path.split("?")[0]} returned HTTP ${response.status}`)
    error.status = response.status
    throw error
  }
  return response.status === 204 ? null : response.json()
}
export async function pages(path, key) {
  const all = []
  for (let page = 1; ; page++) {
    const value = await api(`${path}${path.includes("?") ? "&" : "?"}per_page=100&page=${page}`)
    const rows = key ? value[key] : value
    all.push(...rows)
    if (rows.length < 100) return all
  }
}
export async function protectedMain(repo, sha) {
  const metadata = await api(`repos/${repo}`)
  if (metadata.default_branch !== "main" || !metadata.private || metadata.fork) throw new Error("expected standalone private repository with default main")
  const branch = await api(`repos/${repo}/branches/main`)
  if (!branch.protected || branch.commit.sha !== sha) throw new Error("release source must be the current protected main commit")
  return metadata
}
export async function greenCommit(repo, sha) {
  return checksPassed(await pages(`repos/${repo}/commits/${sha}/check-runs?filter=latest`, "check_runs"))
}
export async function dispatchChecks(repo, branch) {
  for (const workflow of ["ci.yml", "codeql.yml"]) {
    const runs = await api(`repos/${repo}/actions/workflows/${workflow}/runs?branch=${encodeURIComponent(branch)}&per_page=10`)
    const ref = await api(`repos/${repo}/git/ref/heads/${branch}`)
    if (runs.workflow_runs.some((run) => run.head_sha === ref.object.sha)) continue
    await api(`repos/${repo}/actions/workflows/${workflow}/dispatches`, { method: "POST", body: { ref: branch } })
  }
}
export async function enableAutoMerge(repo, pr) {
  const args = protectedMergeArguments(repo, pr)
  const metadata = await api(`repos/${repo}`)
  const base = await api(`repos/${repo}/branches/${pr.base.ref}`)
  if (!metadata.allow_auto_merge || !base.protected) throw new Error("native auto-merge requires enabled repository auto-merge and a protected base branch")
  if (pr.draft || pr.head.repo?.full_name !== repo) throw new Error("only internal ready pull requests are eligible")
  if (pr.auto_merge) return
  // The supported CLI handles both pending checks and an already-clean PR.
  // No admin bypass: GitHub enforces every rule against this exact head.
  execFileSync("gh", args, { stdio: "inherit" })
}
