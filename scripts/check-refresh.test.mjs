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

function codeQLCheck(name, conclusion, id) {
  return { id, name, app: { slug: "github-actions" }, status: "completed", conclusion }
}

function pageNumber(path) {
  const match = /[?&]page=(\d+)/.exec(path)
  return match ? Number(match[1]) : 1
}

function mockRequest({ ciConclusion, codeqlConclusion, body = "Prepare Darkphish 0.7.1", checks = [], ciPages, codeqlPages, checkPages }) {
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
      const page = pageNumber(path)
      return { workflow_runs: ciPages ? (ciPages[page - 1] || []) : page === 1 ? [{ head_sha: sha, status: "completed", conclusion: ciConclusion }] : [] }
    }
    if (path === `repos/${repo}/actions/workflows/ci.yml/dispatches`) return {}
    if (path.startsWith(`repos/${repo}/actions/workflows/codeql.yml/runs?`)) {
      const page = pageNumber(path)
      return { workflow_runs: codeqlPages ? (codeqlPages[page - 1] || []) : page === 1 ? [{ head_sha: sha, status: "completed", conclusion: codeqlConclusion }] : [] }
    }
    if (path.startsWith(`repos/${repo}/commits/${sha}/check-runs?filter=latest`)) {
      const page = pageNumber(path)
      return { check_runs: checkPages ? (checkPages[page - 1] || []) : page === 1 ? checks : [] }
    }
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

test("preserves a failed on-demand CodeQL check instead of refreshing it away", async () => {
  const checks = [
    codeQLCheck("CodeQL (go)", "failure", 20),
    codeQLCheck("CodeQL (javascript-typescript)", "success", 21),
  ]
  const { calls, request } = mockRequest({ ciConclusion: "success", codeqlConclusion: "action_required", checks })
  const result = await dispatchChecks(repo, branch, { request })
  assert.deepEqual(result, { sha, dispatched: false })
  assert.equal(calls.some(({ path }) => path === `repos/${repo}/dispatches`), false)
})

test("refreshes CodeQL only when a required matching check is genuinely absent", async () => {
  const checks = [codeQLCheck("CodeQL (go)", "success", 30)]
  const { calls, request } = mockRequest({ ciConclusion: "success", codeqlConclusion: "action_required", checks })
  const result = await dispatchChecks(repo, branch, { request })
  assert.deepEqual(result, { sha, dispatched: true })
  assert.ok(calls.some(({ path, options }) => path === `repos/${repo}/dispatches` && options.method === "POST" && options.body?.event_type === "generated-release-codeql"))
})

test("does not refresh when an exact-head workflow failure exists on a later page", async () => {
  const unrelated = Array.from({ length: 100 }, (_, index) => ({ head_sha: String(index).padStart(40, "b"), status: "completed", conclusion: "success" }))
  const ciPages = [unrelated, [{ head_sha: sha, status: "completed", conclusion: "failure" }]]
  const codeqlPages = [unrelated, [{ head_sha: sha, status: "completed", conclusion: "failure" }]]
  const { calls, request } = mockRequest({ ciConclusion: "success", codeqlConclusion: "success", ciPages, codeqlPages })
  const result = await dispatchChecks(repo, branch, { request })
  assert.deepEqual(result, { sha, dispatched: false })
  assert.equal(calls.some(({ options }) => options.method === "POST"), false)
})

test("preserves a failed required CodeQL check found on a later page", async () => {
  const unrelated = Array.from({ length: 100 }, (_, index) => ({ id: index + 1, name: `Other ${index}`, app: { slug: "github-actions" }, status: "completed", conclusion: "success" }))
  const checkPages = [unrelated, [
    codeQLCheck("CodeQL (go)", "failure", 1001),
    codeQLCheck("CodeQL (javascript-typescript)", "success", 1002),
  ]]
  const { calls, request } = mockRequest({ ciConclusion: "success", codeqlConclusion: "action_required", checkPages })
  const result = await dispatchChecks(repo, branch, { request })
  assert.deepEqual(result, { sha, dispatched: false })
  assert.equal(calls.some(({ path }) => path === `repos/${repo}/dispatches`), false)
})

test("preserves an older failed CodeQL check even when a newer check with the same name succeeds", async () => {
  const checks = [
    codeQLCheck("CodeQL (go)", "failure", 40),
    codeQLCheck("CodeQL (go)", "success", 41),
  ]
  const { calls, request } = mockRequest({ ciConclusion: "success", codeqlConclusion: "action_required", checks })
  const result = await dispatchChecks(repo, branch, { request })
  assert.deepEqual(result, { sha, dispatched: false })
  assert.equal(calls.some(({ path }) => path === `repos/${repo}/dispatches`), false)
})