import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import vm from "node:vm"

const workflow = readFileSync(new URL("../workflows/release-recover.yml", import.meta.url), "utf8")
const guard = readFileSync(new URL("./release-missing-tag-guard.mjs", import.meta.url), "utf8")

function fixture(ids = [12]) {
  const sha = "a".repeat(40), events = [], releases = ids.map(id => ({id, tag_name: "v0.13.0", draft: false, prerelease: false}))
  const state = {sha, events, releases, allowed: true, ambiguous: false, tag: null, outputs: []}
  const context = vm.createContext({
    URL, fileURLToPath: url => url.pathname, console: {warn() {}},
    process: {execPath: "node-test", env: {RECOVERY_EXECUTION_SHA: sha, GITHUB_OUTPUT: "memory"}},
    readFileSync: () => "0.13.0", appendFileSync: (_, text) => state.outputs.push(text),
    repository: () => "owner/repo", versionTag: v => `v${v}`,
    setTimeout: fn => {assert.ok(events.length < 20, "bounded mock retry"); fn()},
    execFileSync: (binary, args, options) => {
      assert.equal(binary, "node-test"); assert.equal(args[1], "execution")
      assert.ok(args[0].endsWith("/release-publication-guard.mjs"))
      assert.equal(options.env.RECOVERY_EXECUTION_SHA, sha)
      assert.equal(options.stdio, "pipe"); events.push("authority")
      if (!state.allowed) throw Error("protected main moved or required checks unavailable")
    },
    pages: async () => releases.filter(r => !r.draft).map(r => ({...r})),
    api: async (path, options = {}) => {
      if (path.includes("/git/ref/tags/")) return state.tag
      const release = path.includes("/releases/tags/") ? releases.find(r => !r.draft) : releases.find(r => path.endsWith(`/${r.id}`))
      if (options.method === "PATCH") {
        assert.equal(events.at(-1), "authority"); events.push(`patch:${release.id}`)
        assert.equal(options.body.draft, true)
        if (state.ambiguous) {state.allowed = false; throw Error("ambiguous response")}
        release.draft = true
        if (state.stopAfterFirst) state.allowed = false
      }
      return release ? {...release} : null
    },
  })
  vm.runInContext(guard.replace(/^import .*\r?\n/gm, "").split("main().catch")[0].replaceAll("import.meta.url", JSON.stringify("file:///trusted/release-missing-tag-guard.mjs")), context)
  return {state, run: code => vm.runInContext(code || "main()", context)}
}

test("tagless workflow supplies immutable SHA and every target invokes the trusted execution gate", async () => {
  assert.match(workflow, /name: Withdraw tagless public release fail closed\r?\n\s+env:\r?\n\s+GH_TOKEN:.*\r?\n\s+RECOVERY_EXECUTION_SHA: \$\{\{ github.sha \}\}/)
  const f = fixture([12, 13]); await f.run()
  assert.deepEqual(f.state.events, ["authority", "patch:12", "authority", "patch:13"])
  assert.deepEqual(f.state.outputs, ["tag_absent=true\n"])
})

test("stale execution and absent SHA abort without withdrawal or absent-tag output", async () => {
  const stale = fixture(); stale.state.allowed = false
  await assert.rejects(stale.run(), /protected main moved/)
  assert.deepEqual(stale.state.events, ["authority"]); assert.deepEqual(stale.state.outputs, [])
  const invalid = fixture(); invalid.run("process.env.RECOVERY_EXECUTION_SHA = undefined")
  await assert.rejects(invalid.run(), /execution SHA is invalid/)
  assert.deepEqual(invalid.state.events, []); assert.equal(invalid.state.releases[0].draft, false)
})

test("authority failure is not swallowed as an ambiguous PATCH on next target or retry", async () => {
  for (const retry of [false, true]) {
    const f = fixture(retry ? [12] : [12, 13])
    f.state.ambiguous = retry; f.state.stopAfterFirst = !retry
    await assert.rejects(f.run(), /protected main moved/)
    assert.deepEqual(f.state.events, ["authority", "patch:12", "authority"])
    assert.deepEqual(f.state.outputs, [])
    assert.equal(f.state.releases.at(-1).draft, false)
  }
})

test("tagless public releases are withdrawn before publication provenance preflight", () => {
  const executionGuard = workflow.indexOf("Verify immutable recovery execution before any release mutation")
  const missingTagGuard = workflow.indexOf("Withdraw tagless public release fail closed")
  const publicationGuard = workflow.indexOf("Preserve only cryptographically verified public releases")
  assert.ok(executionGuard > 0 && missingTagGuard > executionGuard && publicationGuard > missingTagGuard)
  assert.match(workflow, /node \.github\/scripts\/release-publication-guard\.mjs execution/)
  assert.match(workflow, /node \.github\/scripts\/release-missing-tag-guard\.mjs/)
  assert.match(guard, /release\?\.tag_name === tag && release\.draft === false/)
  assert.match(guard, /draft: true/)
  assert.match(guard, /tag_absent/)
  assert.match(guard, /releases\/tags\/\$\{tag\}/)
  assert.match(guard, /release tag appeared while withdrawing tagless public release/)
})

test("tag appearance never prevents confirmed withdrawal and cannot become a recovery baseline", () => {
  const patch = guard.indexOf('method: "PATCH"')
  const tagProbe = guard.indexOf("await currentTag(repo, tag)")
  const appearedFailure = guard.indexOf("release tag appeared while withdrawing tagless public release")
  assert.ok(patch >= 0 && tagProbe >= 0 && appearedFailure > patch)
  assert.match(guard, /const finalDirect = await directSnapshot/)
  assert.match(guard, /directNotWithdrawn/)
  assert.match(guard, /release\.draft !== true \|\| release\.prerelease !== false/)
  assert.match(guard, /publicByTag/)
  assert.match(guard, /output\("tag_absent", "true"\)/)
})

test("present tag precheck exports exact immutable ref object identity", () => {
  assert.match(guard, /\["commit", "tag"\]\.includes\(objectType\)/)
  assert.match(guard, /`false:\$\{tagState\.objectType\}:\$\{tagState\.objectSha\}`/)
  assert.match(guard, /output\("tag_absent", encodedTagSnapshot\(tagState\)\)/)
})
