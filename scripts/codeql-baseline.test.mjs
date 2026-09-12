import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"
import { summarizeAlerts, verifyCodeQLBaseline } from "./codeql-baseline.mjs"

const repo = "owner/repository", sha = "a".repeat(40), ref = "refs/heads/main"
const analysis = (language, overrides = {}) => ({ ref, commit_sha: sha, tool: { name: "CodeQL" }, category: `.github/workflows/codeql.yml:analyze-main/language:${language}`, error: "", warning: "", rules_count: 87, ...overrides })
const alert = (number, severity = "high") => ({ number, state: "open", tool: { name: "CodeQL" }, rule: { id: "go/example", security_severity_level: severity }, most_recent_instance: { ref, state: "open", location: { path: "synthetic.go" } }, message: "synthetic-do-not-print" })

function fixture({ alerts = [], analyses = [analysis("go"), analysis("javascript-typescript")], fail, moved = false, unprotected = false } = {}) {
  const calls = [], output = []
  let branchReads = 0
  const get = async (path) => {
    calls.push(path)
    if (fail?.(path)) throw new Error("synthetic API unavailable")
    if (path === `repos/${repo}`) return { default_branch: "main" }
    if (path === `repos/${repo}/branches/main`) return { protected: !unprotected, commit: { sha: moved && branchReads++ > 0 ? "b".repeat(40) : sha } }
    const url = new URL(`https://api.github.com/${path}`)
    assert.equal(url.searchParams.get("ref"), ref)
    assert.equal(url.searchParams.get("tool_name"), "CodeQL")
    const page = Number(url.searchParams.get("page"))
    assert.equal(url.searchParams.get("per_page"), "100")
    if (url.pathname.endsWith("/alerts")) {
      assert.equal(url.searchParams.get("state"), "open")
      return alerts.slice((page - 1) * 100, page * 100)
    }
    assert.equal(url.searchParams.get("direction"), "desc")
    return analyses.slice((page - 1) * 100, page * 100)
  }
  return { calls, output, get, log: (value) => output.push(value) }
}

test("clean current-source analyses yield only the permitted zero-count diagnostic", async () => {
  const fake = fixture()
  assert.equal((await verifyCodeQLBaseline(repo, sha, fake)).total_open, 0)
  assert.deepEqual(JSON.parse(fake.output[0]), { total_open: 0, critical: 0, high: 0, medium: 0, low: 0, warning: 0, alerts: [] })
})

test("every open severity blocks and alert messages are never reported", async () => {
  for (const severity of ["critical", "high", "medium", "low", null]) {
    const fake = fixture({ alerts: [alert(1, severity)] })
    await assert.rejects(verifyCodeQLBaseline(repo, sha, fake), /unresolved CodeQL/)
    const report = JSON.parse(fake.output[0])
    assert.equal(report.total_open, 1)
    assert.equal(report[severity ?? "warning"], 1)
    assert.deepEqual(report.alerts, [{ number: 1, rule: "go/example", path: "synthetic.go" }])
    assert.equal(fake.output[0].includes("synthetic-do-not-print"), false)
  }
})

test("all alert pages are inspected, including beyond the first hundred", async () => {
  const fake = fixture({ alerts: Array.from({ length: 101 }, (_, i) => alert(i + 1, i === 100 ? "critical" : "low")) })
  await assert.rejects(verifyCodeQLBaseline(repo, sha, fake), /unresolved CodeQL/)
  assert.equal(JSON.parse(fake.output[0]).critical, 1)
  assert.equal(JSON.parse(fake.output[0]).total_open, 101)
  assert.ok(fake.calls.some((path) => path.includes("/alerts?") && path.endsWith("page=2")))
})

test("missing, stale, failed, empty-rule and wrong-ref analyses fail closed", async () => {
  for (const overrides of [{ commit_sha: "b".repeat(40) }, { error: "extraction failed" }, { warning: "partial extraction" }, { rules_count: 0 }, { ref: "refs/pull/1/merge" }, { tool: { name: "untrusted" } }]) {
    const fake = fixture({ analyses: [analysis("go", overrides), analysis("javascript-typescript")] })
    await assert.rejects(verifyCodeQLBaseline(repo, sha, fake), /successful current-source/)
  }
  await assert.rejects(verifyCodeQLBaseline(repo, sha, fixture({ analyses: [] })), /successful current-source/)
  const fake = fixture({ analyses: [analysis("go", { error: "new failure" }), analysis("go"), analysis("javascript-typescript")] })
  await assert.rejects(verifyCodeQLBaseline(repo, sha, fake), /successful current-source/)
})

test("analysis pagination finds both latest configured languages", async () => {
  const unrelated = Array.from({ length: 100 }, () => analysis("other"))
  const fake = fixture({ analyses: [...unrelated, analysis("go"), analysis("javascript-typescript")] })
  assert.equal((await verifyCodeQLBaseline(repo, sha, fake)).total_open, 0)
  assert.ok(fake.calls.some((path) => path.includes("/analyses?") && path.endsWith("page=2")))
})

test("API errors, malformed pages and source/protection changes cannot pass", async () => {
  for (const part of ["/branches/", "/analyses?", "/alerts?"]) await assert.rejects(verifyCodeQLBaseline(repo, sha, fixture({ fail: (path) => path.includes(part) })), /unavailable/)
  for (const options of [{ moved: true }, { unprotected: true }]) await assert.rejects(verifyCodeQLBaseline(repo, sha, fixture(options)), /branch/)
  const fake = fixture(), original = fake.get
  fake.get = (path) => path.includes("/alerts?") ? {} : original(path)
  await assert.rejects(verifyCodeQLBaseline(repo, sha, fake), /invalid page/)
  await assert.rejects(verifyCodeQLBaseline("../outside", sha, fixture()), /Invalid/)
})

test("malformed, duplicated or wrong-branch alert records are rejected", () => {
  for (const value of [{}, { ...alert(1), state: "dismissed" }, { ...alert(1), rule: { id: "go/example", security_severity_level: "unknown" } }, { ...alert(1), most_recent_instance: { ref: "refs/heads/other" } }]) assert.throws(() => summarizeAlerts([value], ref))
  assert.throws(() => summarizeAlerts([alert(1), alert(1)], ref), /inconsistent/)
})

test("the production diagnostic makes GET-only authenticated requests and rejects permission errors", async () => {
  const originalFetch = globalThis.fetch, originalToken = process.env.GH_TOKEN
  process.env.GH_TOKEN = "synthetic-read-token"
  try {
    const fake = fixture()
    globalThis.fetch = async (url, options) => {
      assert.equal(options.method, "GET"); assert.equal(options.redirect, "error"); assert.ok(options.signal); assert.equal(new URL(url).origin, "https://api.github.com")
      return new Response(JSON.stringify(await fake.get(new URL(url).pathname.slice(1) + new URL(url).search)))
    }
    await verifyCodeQLBaseline(repo, sha, { log: () => {} })
    for (const status of [403, 404, 503]) { globalThis.fetch = async () => new Response("synthetic-private-response", { status }); await assert.rejects(verifyCodeQLBaseline(repo, sha), new RegExp(`HTTP ${status}`)) }
    globalThis.fetch = async () => new Response("synthetic-private-response")
    await assert.rejects(verifyCodeQLBaseline(repo, sha), /invalid JSON/)
  } finally {
    globalThis.fetch = originalFetch
    if (originalToken === undefined) delete process.env.GH_TOKEN
    else process.env.GH_TOKEN = originalToken
  }
})

test("preparation and publication require the read-only gate without changing required checks", () => {
  const prepare = readFileSync(new URL("./release-prepare.mjs", import.meta.url), "utf8"), publish = readFileSync(new URL("./release-publish.mjs", import.meta.url), "utf8")
  assert.ok(prepare.indexOf("await verifyCodeQLBaseline(repo, sha)") < prepare.indexOf('git("push"'))
  assert.ok((publish.match(/await verifyCodeQLBaseline\(repo, sha\)/g) || []).length >= 2)
  for (const name of ["release-prepare.yml", "release.yml", "security-baseline.yml"]) {
    const workflow = readFileSync(new URL(`../.github/workflows/${name}`, import.meta.url), "utf8")
    assert.match(workflow, /security-events: read/); assert.doesNotMatch(workflow, /security-events: write/)
  }
})
