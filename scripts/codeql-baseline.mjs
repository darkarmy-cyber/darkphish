import { pathToFileURL } from "node:url"
import { git, repository } from "./release-lib.mjs"

const categories = ["go", "javascript-typescript"].map((language) => `.github/workflows/codeql.yml:analyze/language:${language}`)

// Deliberately GET-only; a read-scoped token suffices. Never follow redirects
// with credentials and never print response bodies, alert messages or secrets.
async function readGitHub(path) {
  const token = process.env.GH_TOKEN || process.env.GITHUB_TOKEN
  if (!token) throw new Error("CodeQL baseline requires an authenticated read token")
  const response = await fetch(`https://api.github.com/${path}`, {
    method: "GET", redirect: "error", signal: AbortSignal.timeout(15000),
    headers: { Accept: "application/vnd.github+json", Authorization: `Bearer ${token}`, "X-GitHub-Api-Version": "2026-03-10" },
  })
  if (!response.ok) throw new Error(`CodeQL baseline read failed: HTTP ${response.status}; verify Code scanning alerts read permission and service availability`)
  try { return await response.json() } catch { throw new Error("CodeQL baseline received invalid JSON") }
}

async function pages(get, path, visit) {
  for (let page = 1; page <= 100; page++) {
    const rows = await get(`${path}&per_page=100&page=${page}`)
    if (!Array.isArray(rows) || rows.length > 100) throw new Error("CodeQL baseline received an invalid page")
    if (visit(rows) || rows.length < 100) return
  }
  throw new Error("CodeQL baseline pagination limit reached; manual investigation required")
}

export function summarizeAlerts(alerts, ref) {
  const report = { total_open: 0, critical: 0, high: 0, medium: 0, low: 0, warning: 0, alerts: [] }
  const seen = new Set()
  for (const alert of alerts) {
    const instance = alert?.most_recent_instance
    if (alert?.state !== "open" || alert.tool?.name !== "CodeQL" || instance?.ref !== ref || instance.state !== "open" ||
        !Number.isSafeInteger(alert.number) || alert.number < 1 || seen.has(alert.number) ||
        typeof alert.rule?.id !== "string" || !alert.rule.id || typeof instance.location?.path !== "string" || !instance.location.path) {
      throw new Error("CodeQL baseline received an inconsistent alert record")
    }
    const severity = alert.rule.security_severity_level ?? "warning"
    if (!["critical", "high", "medium", "low", "warning"].includes(severity)) throw new Error("CodeQL baseline received an unknown severity")
    seen.add(alert.number)
    report.total_open++
    report[severity]++
    report.alerts.push({ number: alert.number, rule: alert.rule.id, path: instance.location.path })
  }
  report.alerts.sort((a, b) => a.number - b.number)
  return report
}

export async function verifyCodeQLBaseline(repo, expectedSHA, { get = readGitHub, log = console.log } = {}) {
  if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repo) || repo.split("/").some((part) => part === "." || part === "..") ||
      !/^[a-f0-9]{40}$/.test(expectedSHA || "")) throw new Error("Invalid CodeQL baseline source")
  const metadata = await get(`repos/${repo}`)
  if (typeof metadata?.default_branch !== "string" || !metadata.default_branch) throw new Error("Cannot verify the default branch")
  const ref = `refs/heads/${metadata.default_branch}`
  const branchPath = `repos/${repo}/branches/${encodeURIComponent(metadata.default_branch)}`
  const branch = await get(branchPath)
  if (branch?.commit?.sha !== expectedSHA || branch.protected !== true) throw new Error("CodeQL baseline source is not the current protected default branch")

  const latest = new Map()
  await pages(get, `repos/${repo}/code-scanning/analyses?ref=${encodeURIComponent(ref)}&tool_name=CodeQL&sort=created&direction=desc`, (rows) => {
    for (const analysis of rows) {
      if (categories.includes(analysis?.category) && !latest.has(analysis.category)) latest.set(analysis.category, analysis)
    }
    return latest.size === categories.length
  })
  for (const category of categories) {
    const analysis = latest.get(category)
    if (analysis?.ref !== ref || analysis.commit_sha !== expectedSHA || analysis.tool?.name !== "CodeQL" ||
        analysis.error !== "" || analysis.warning || !Number.isSafeInteger(analysis.rules_count) || analysis.rules_count <= 0) {
      throw new Error(`CodeQL baseline requires a successful current-source ${category.split(":").at(-1)} analysis; dispatch CodeQL on the default branch`)
    }
  }

  const alerts = []
  await pages(get, `repos/${repo}/code-scanning/alerts?ref=${encodeURIComponent(ref)}&tool_name=CodeQL&state=open`, (rows) => { alerts.push(...rows); return false })
  const report = summarizeAlerts(alerts, ref)
  const finalBranch = await get(branchPath)
  const finalMetadata = await get(`repos/${repo}`)
  if (finalBranch?.commit?.sha !== expectedSHA || finalBranch.protected !== true || finalMetadata?.default_branch !== metadata.default_branch) {
    throw new Error("Default branch changed during CodeQL baseline verification; retry against current source")
  }
  log(JSON.stringify(report))
  if (report.total_open !== 0) throw new Error("Release blocked: unresolved CodeQL alerts; remediate or individually prove and review false positives")
  return report
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  verifyCodeQLBaseline(repository(), git("rev-parse", "HEAD")).catch((error) => { console.error(error.message); process.exitCode = 1 })
}
