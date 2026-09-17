// GitBook previews are informational. This narrow exception never replaces
// required checks, review gates, branch protection, or an exact-head merge.
const contexts = new Set(['GitBook (./docs)', 'GitBook (./doc)', 'GitBook (./changes)'])
export function informationalGitBook(status) {
  return contexts.has(status.context) && status.creator?.login === 'gitbook-com[bot]' &&
    status.creator?.id === 92167642 && status.creator?.type === 'Bot'
}

export async function mergeStateReady(repo, pr, {request, pages}) {
  if (pr.mergeable !== true) return false
  if (!['clean', 'unstable'].includes(pr.mergeable_state)) return false
  const prefix = `repos/${repo}`
  const checks = await pages(`${prefix}/commits/${pr.head.sha}/check-runs?filter=latest`, 'check_runs', request)
  // Includes extra executed workflows, not merely the required names. In
  // particular, a failed dispatched CI must not be hidden by a passing PR CI.
  if (!checks.length || checks.some(c => c.status !== 'completed' || !['success', 'skipped', 'neutral'].includes(c.conclusion))) return false
  if (pr.mergeable_state === 'clean') return true
  // Do not exempt a GitBook check that a maintainer explicitly made required.
  // The branch summary is available with contents:read; the administrative
  // protection endpoint is not available to a normal Actions token.
  const branch = await request(`${prefix}/branches/main`)
  const protection = branch.protection?.required_status_checks
  if (branch.protected !== true || !protection || !Array.isArray(protection.contexts) || !Array.isArray(protection.checks)) return false
  const rules = await pages(`${prefix}/rules/branches/main`, undefined, request)
  const required = [
    ...(protection.contexts || []), ...(protection.checks || []).map(c => c.context),
    ...rules.filter(r => r.type === 'required_status_checks').flatMap(r => r.parameters.required_status_checks.map(c => c.context)),
  ]
  if (required.some(name => contexts.has(name))) return false
  const statuses = await pages(`${prefix}/commits/${pr.head.sha}/statuses`, undefined, request)
  const latest = new Map()
  for (const s of statuses) {
    if (!Number.isSafeInteger(s.id) || typeof s.context !== 'string') return false
    if (!latest.has(s.context) || latest.get(s.context).id < s.id) latest.set(s.context, s)
  }
  const nonGreen = [...latest.values()].filter(s => s.state !== 'success')
  return nonGreen.length > 0 && nonGreen.every(s => informationalGitBook(s) && ['pending', 'failure', 'error'].includes(s.state))
}
