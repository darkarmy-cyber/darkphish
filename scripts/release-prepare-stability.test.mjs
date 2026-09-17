import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"
import { posix } from "node:path"
import vm from "node:vm"

const prepareSource = readFileSync(new URL("release-prepare.mjs", import.meta.url), "utf8")
  .replace(/^import .*\r?\n/gm, "").replace(/prepare\(\)\.catch\([^\n]+\)\s*$/, "")
const generatorSource = readFileSync(new URL("changelog.mjs", import.meta.url), "utf8")
  .replace(/^import .*\r?\n/gm, "").replaceAll("import.meta.url", JSON.stringify("file:///virtual/scripts/changelog.mjs"))
const fixture = () => new Map([
  ["VERSION", "0.11.0\n"],
  ["CHANGELOG.md", "# Changelog\n\n## 0.11.0 - 2026-09-17\n\n### Fixed\n\n- History.\n"],
  ["changes/fix.md", "---\ncategory: Fixed\nversion: 0.12.0\n---\n- Fix release preparation.\n"],
])
const serialize = memory => JSON.stringify([...memory].sort(([a], [b]) => a.localeCompare(b)))
const fixedDate = value => class extends Date { constructor(...args) { super(...(args.length ? args : [value])) } }

// Execute the real generator with only its filesystem redirected to memory.
function generate(memory, command, now) {
  let output = ""
  const key = path => path.replace(/^\/virtual\//, "")
  const context = {
    URL, join: posix.join, fileURLToPath: () => "/virtual/", Date: fixedDate(now),
    readFileSync: path => { assert.ok(memory.has(key(path))); return memory.get(key(path)) },
    writeFileSync: (path, value) => memory.set(key(path), value),
    unlinkSync: path => assert.ok(memory.delete(key(path))),
    readdirSync: () => [...memory.keys()].filter(path => path.startsWith("changes/"))
      .map(path => ({ name: path.slice(8), isFile: () => true })),
    execFileSync: () => { throw Error("Unexpected generator subprocess") },
    process: { argv: ["node", "changelog.mjs", command], stdout: { write: value => { output += value } }, stderr: { write: message => { throw Error(message) } } },
  }
  vm.runInNewContext(generatorSource, context, { timeout: 5000 })
  assert.notEqual(context.process.exitCode, 1)
  return output
}

async function runPreparation(options = {}) {
  const now = options.now || "2026-09-19T00:00:01Z", creation = options.creation || (options.fresh ? now : "2026-09-18T23:59:59Z")
  const historical = fixture()
  if (options.existingHeading) {
    historical.set("VERSION", "0.12.0\n")
    historical.set("CHANGELOG.md", historical.get("CHANGELOG.md").replace("# Changelog\n", "# Changelog\n\n## 0.12.0 - 2026-09-12\n\n### Fixed\n\n- Earlier unreleased history.\n"))
  }
  const branch = new Map(historical), current = new Map(historical)
  generate(branch, "prepare", creation)
  if (options.tamper) branch.set("CHANGELOG.md", branch.get("CHANGELOG.md") + "Untrusted change.\n")
  if (options.newFragment) current.set("changes/extra.md", "---\ncategory: Fixed\nversion: 0.12.0\n---\n- Additional fix.\n")
  let temporary, pushes = 0, commits = 0, merges = 0, refreshes = 0, branchValidations = 0
  const operations = []
  const oldSHA = "b".repeat(40), sha = "a".repeat(40)
  const pr = { number: 99, state: "open", draft: false, base: { ref: "main" }, head: { ref: "release/v0.12.0", repo: { full_name: "owner/repo" } }, title: "release: Darkphish 0.12.0", user: { login: "github-actions[bot]", id: 41898282, type: "Bot" }, created_at: creation, ...options.pr }
  const select = path => path.startsWith("/temporary/") ? [temporary, path.slice(11)] : [current, path.replace(/^\.\//, "")]
  const context = {
    Date: fixedDate(now), join: posix.join, tmpdir: () => "/temporary", console: { log: () => {} },
    process: { execPath: "node" }, repository: () => "owner/repo", versionTag: value => `v${value}`,
    protectedMain: async () => {}, assertReleaseAdvancePublished: async () => {}, greenCommit: async () => true,
    verifyCodeQLBaseline: async () => {}, assertGeneratedCommits: () => { branchValidations++ },
    readFileSync: path => { const [memory, key] = select(path); assert.ok(memory.has(key), key); return memory.get(key) },
    writeFileSync: (path, value) => { const [memory, key] = select(path); memory.set(key, value) },
    readdirSync: () => [...current.keys()].filter(path => path.startsWith("changes/")).map(path => path.slice(8)),
    mkdtempSync: () => "/temporary", copyFileSync: () => {}, rmSync: () => {},
    pages: async () => options.fresh ? [] : [pr],
    api: async (path, settings = {}) => {
      if (path.includes("git/ref/tags/") || path.includes("releases/tags/")) return null
      if (path.includes("git/ref/heads/")) return options.fresh ? null : { object: { sha: oldSHA } }
      if (path.endsWith("actions/permissions/workflow")) return { can_approve_pull_request_reviews: true }
      if (path.endsWith("/pulls") && settings.method === "POST") return pr
      if (path.endsWith("/labels")) return {}
      throw Error(`Unexpected API ${path}`)
    },
    dispatchChecks: async () => { refreshes++; operations.push("checks"); return { dispatched: true, sha: oldSHA } },
    mergeReviewedPullRequest: async () => { merges++; throw Error("No merge while refreshed checks pending") },
    git: (...args) => {
      const [command, ...rest] = args
      if (command === "fetch" || command === "config" || command === "add") return ""
      if (command === "rev-parse") return rest[0] === "HEAD" ? sha : serialize(branch)
      if (command === "merge-base") return "c".repeat(40)
      if (command === "log") return "github-actions[bot]\trelease: Darkphish 0.12.0"
      if (command === "diff") return "VERSION\nCHANGELOG.md\nchanges/fix.md"
      if (command === "worktree") { if (rest[0] === "add") temporary = new Map(historical); return "" }
      if (command === "write-tree") return serialize(current)
      if (command === "commit-tree") { commits++; operations.push("commit"); return "d".repeat(40) }
      if (command === "push") { pushes++; operations.push("push"); return "" }
      throw Error(`Unexpected git ${args.join(" ")}`)
    },
    execFileSync: (binary, args, settings = {}) => {
      if (binary === "node") return generate(settings.cwd === "/temporary" ? temporary : current, args[1], now)
      assert.equal(binary, "git"); assert.equal(args[0], "diff")
      return serialize(settings.cwd === "/temporary" ? temporary : branch)
    },
  }
  vm.runInNewContext(prepareSource, context, { timeout: 5000 })
  await vm.runInNewContext("prepare()", context)
  return { current, pushes, commits, merges, refreshes, branchValidations, operations }
}

test("existing release preparation preserves exact tree across UTC days without a push", async () => {
  const first = await runPreparation({ now: "2026-09-18T23:59:59Z" })
  const second = await runPreparation({ now: "2026-09-19T00:00:01Z" })
  const later = await runPreparation({ now: "2026-09-22T12:00:00Z" })
  for (const result of [first, second, later]) {
    assert.equal(serialize(result.current), serialize(first.current))
    assert.equal(result.pushes, 0); assert.equal(result.commits, 0)
    assert.equal(result.merges, 0); assert.equal(result.refreshes, 1); assert.equal(result.branchValidations, 1)
    assert.match(result.current.get("CHANGELOG.md"), /## 0\.12\.0 - 2026-09-18/)
  }
})

test("new legitimate fragments change the tree while retaining the trusted date", async () => {
  const result = await runPreparation({ newFragment: true })
  assert.equal(result.pushes, 1); assert.equal(result.commits, 1)
  assert.match(result.current.get("CHANGELOG.md"), /Additional fix/)
  assert.match(result.current.get("CHANGELOG.md"), /## 0\.12\.0 - 2026-09-18/)
})

test("modified generated branch is rejected rather than normalized into trusted content", async () => {
  await assert.rejects(runPreparation({ tamper: true }), /differs from trusted generated output/)
})

test("missing, malformed and impossible server dates fail closed", async () => {
  for (const created_at of [undefined, "", "2026-09-18", "2026-02-30T00:00:00Z", "2026-13-01T00:00:00Z", "2026-09-18T25:00:00Z", "2026-09-18T00:00:00Zgarbage", "2026-09-18T00:00:00+02:00"])
    await assert.rejects(runPreparation({ pr: { created_at } }), /valid server creation date/)
})

test("spoofed PR identity cannot supply the release date", async () => {
  for (const pr of [{ user: { login: "github-actions[bot]", id: 1, type: "Bot" } }, { draft: true }, { state: "closed" }, { title: "release: Other" }, { head: { ref: "release/v0.12.0", repo: { full_name: "other/repo" } } }])
    await assert.rejects(runPreparation({ pr }), /not the trusted generated release/)
})

test("new release agrees with same-day server date and still waits for refreshed checks", async () => {
  const result = await runPreparation({ fresh: true })
  assert.match(result.current.get("CHANGELOG.md"), /## 0\.12\.0 - 2026-09-19/)
  assert.equal(result.commits, 1); assert.equal(result.pushes, 1); assert.equal(result.merges, 0)
})

test("first creation crossing UTC midnight aligns to server date before checks", async () => {
  const creation = "2026-09-19T00:00:01Z"
  const first = await runPreparation({ fresh: true, now: "2026-09-18T23:59:59Z", creation })
  const retry = await runPreparation({ now: "2026-09-20T12:00:00Z", creation })
  assert.equal(serialize(first.current), serialize(retry.current))
  assert.match(first.current.get("CHANGELOG.md"), /## 0\.12\.0 - 2026-09-19/)
  assert.equal(first.commits, 2); assert.equal(first.pushes, 2); assert.equal(first.refreshes, 1)
  assert.deepEqual(first.operations, ["commit", "push", "commit", "push", "checks"])
  assert.equal(retry.commits, 0); assert.equal(retry.pushes, 0)
})

test("invalid creation response cannot trigger date correction or checks", async () => {
  await assert.rejects(runPreparation({ fresh: true, pr: { created_at: "2026-02-30T00:00:00Z" } }), /valid server creation date/)
  await assert.rejects(runPreparation({ fresh: true, pr: { user: { login: "github-actions[bot]", id: 1, type: "Bot" } } }), /not the trusted generated release/)
})

test("current-version aggregation retains its historical section date on creation and retry", async () => {
  const first = await runPreparation({ fresh: true, existingHeading: true, now: "2026-09-18T23:59:59Z", creation: "2026-09-19T00:00:01Z" })
  const retry = await runPreparation({ existingHeading: true, creation: "2026-09-19T00:00:01Z" })
  assert.equal(serialize(first.current), serialize(retry.current))
  assert.match(first.current.get("CHANGELOG.md"), /## 0\.12\.0 - 2026-09-12/)
  assert.match(first.current.get("CHANGELOG.md"), /Earlier unreleased history/)
  assert.equal(first.commits, 1); assert.equal(first.pushes, 1)
  assert.equal(retry.commits, 0); assert.equal(retry.pushes, 0)
})
