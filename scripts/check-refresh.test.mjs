import assert from "node:assert/strict"
import test from "node:test"

import { dispatchChecks } from "./check-refresh.mjs"

const repo = "darkarmy-cyber/darkphish"
const branch = "release/v0.7.1"
const sha = "a".repeat(40)

function releasePull(body = "Prepare Darkphish 0.7.1") {
  return {
    number: 31,
    body,
    draft: false,
    base: { ref: "main" },
    head: { ref: branch, sha, repo: { full_name: repo } },
    title: "release: Darkphish 0.7.1",
  }
}

function mockRequest({ ciConclusion, codeqlConclusion, body = "Prepare Darkphish 0.7.1" }) {
  const calls = []
  let pr = releasePull(body)
  const request = async (path, options = {}) => {
    calls.push({ path, options })
    if (path === `repos/${repo}/git/ref/heads/${branch}`) return { object: { sha } }
    if (path.startsWith(`repos/${repo}/pulls?state=open&base=main&head=`)) return [pr]
    if (path === `repos/${repo}/pulls/${pr.number}` && !options.method) return pr
    if (path === `repos/${repo}/pulls/${pr.number}` && options.method === "PATCH") {
      pr = { ...pr, body: options.body?.body }
      return pr
    }
    if (path.startsWith(`repos/${repo}/actions/workflows/ci.yml/runs?`)) {
      return { workflow_runs: [{ head_sha: sha, status: "completed", conclusion: ciConclusion }] }
    }
    if (path === `repos/${repo}/actions/workflows/ci.yml/dispatches`) return {}
    if (path.startsWith(`repos/${repo}/actions/workflows/codeql.yml/runs?`)) {
      return { workflow_runs: [{ head_sha: sha, status: "completed", conclusion: codeqlConclusion }] }
    }
    if (path === `repos/${repo}/commits/${sha}/check-runs?filter=latest&per_page=100`) return { check_runs: [] }
    if (path === `repos/${repo}/dispatches`) return {}
    throw new Error(`unexpected request: ${path}`)
  }
  return { calls, request, currentPull: () => pr }
}

test("refreshes exact-head release checks that require action before any job ran", async () => {
  const { calls, request, currentPull } = mockRequest({ ciConclusion: "action_required", codeqlConclusion: "action_required" })

  const result = await dispatchChecks(repo, branch, { request })

  assert.deepEqual(result, { sha, dispatched: true })
  assert.ok(calls.some(({ path, options }) => path === `repos/${repo}/actions/workflows/ci.yml/dispatches` && options.method === "POST" && options.body?.ref === branch))
  assert.ok(calls.some(({ path, options }) => path === `repos/${repo}/dispatches` && options.method === "POST" && options.body?.event_type === "generated-release-codeql" && options.body?.client_payload?.sha === sha))
  assert.match(currentPull().body, new RegExp(`darkphish-check-refresh:ci:${sha}`))
  assert.match(currentPull().body, new RegExp(`darkphish-check-refresh:codeql:${sha}`))
})

test("does not repeat a generated-release refresh already recorded for the exact head", async () => {
  const markerBody = `Prepare Darkphish 0.7.1\n\n<!-- darkphish-check-refresh:ci:${sha} -->\n<!-- darkphish-check-refresh:codeql:${sha} -->`
  const { calls, request } = mockRequest({ ciConclusion: "action_required", codeqlConclusion: "action_required", body: markerBody })

  const result = await dispatchChecks(repo, branch, { request })

  assert.deepEqual(result, { sha, dispatched: false })
  assert.equal(calls.some(({ path }) => path === `repos/${repo}/actions/workflows/ci.yml/dispatches`), false)
  assert.equal(calls.some(({ path }) => path === `repos/${repo}/dispatches`), false)
})

test("does not turn a real exact-head workflow failure into an automatic retry loop", async () => {
  const { calls, request } = mockRequest({ ciConclusion: "failure", codeqlConclusion: "failure" })

  const result = await dispatchChecks(repo, branch, { request })

  assert.deepEqual(result, { sha, dispatched: false })
  assert.equal(calls.some(({ options }) => options.method === "POST"), false)
})
