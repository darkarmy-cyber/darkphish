import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { audited0272StaleNativeRun, audited0272Draft, audited0272QuarantinedAsset } from "./release-recover-0272-policy.mjs"
import { recoveryVersion } from "./release-recovery-version.mjs"

const source = "7bfe25c0f122f98196978bdd0ad9b679b65cc8fd"
const staleHead = "c77b486b461c817c8b936edfeb03db1cbce593e3"
const bot = { login: "github-actions[bot]", type: "Bot", id: 41898282 }
const names = [
  [136868091231, "metadata"], [136868172989, "verify"],
  [136868607609, "binaries (ubuntu-24.04-arm, linux, arm64, darkphish)"],
  [136868607646, "binaries (ubuntu-24.04, linux, amd64, darkphish)"],
  [136868607651, "binaries (macos-15-intel, darwin, amd64, darkphish)"],
  [136868607656, "binaries (windows-2025, windows, amd64, darkphish.exe)"],
  [136868607693, "binaries (macos-15, darwin, arm64, darkphish)"],
  [136869616287, "audit-smoke"], [136869835738, "publish"],
]
function incident() {
  return {
    repository: "darkarmy-cyber/darkphish", source,
    run: { id: 44900262093, run_number: 839, run_attempt: 2, workflow_id: 351159930,
      name: "Native release", path: ".github/workflows/release.yml", event: "schedule",
      head_branch: "main", head_sha: staleHead, status: "completed", conclusion: "success",
      created_at: "2026-11-21T11:41:00Z", updated_at: "2026-11-21T14:06:16Z", actor: bot,
      triggering_actor: { login: "oliverkko", id: 309485696, type: "User" } },
    jobs: names.map(([id, name]) => ({ id, name, run_id: 44900262093, run_attempt: 2,
      head_sha: staleHead, status: "completed", conclusion: "success",
      steps: name === "publish" ? ["Generate checksums", "Generate canonical native publication receipt",
        "Attest canonical native release artifacts", "Publish verified assets without overwriting an existing release"]
        .map(name => ({ name, status: "completed", conclusion: "success" })) : [] })),
  }
}

test("only the pinned stale-head native incident can authorize a fresh recovery build", () => {
  const valid = incident()
  assert.equal(audited0272StaleNativeRun(valid), true)
  for (const mutate of [
    x => { x.run.head_sha = source },
    x => { x.run.run_attempt = 3 },
    x => { x.jobs[0].conclusion = "failure" },
    x => { x.jobs[8].steps[3].conclusion = "failure" },
    x => { x.source = staleHead },
    x => { x.repository = "other/darkphish" },
  ]) {
    const changed = incident(); mutate(changed)
    assert.equal(audited0272StaleNativeRun(changed), false)
  }
})

test("only pinned withdrawn release assets can be quarantined", () => {
  const draft = { id: 453734839, tag_name: "v0.27.2", target_commitish: source,
    draft: true, prerelease: false, author: bot }
  const asset = { id: 775775175, name: "SHA256SUMS", size: 610,
    digest: "sha256:40c3d0340663924ca83025117762685d46b27cc2114b065932c24eebf4fab60b",
    state: "uploaded", uploader: bot }
  assert.equal(audited0272Draft(draft, "0.27.2", source), true)
  assert.equal(audited0272QuarantinedAsset(draft, "0.27.2", source, asset), true)
  assert.equal(audited0272QuarantinedAsset(draft, "0.27.2", source, { ...asset, id: asset.id + 1 }), false)
  assert.equal(audited0272QuarantinedAsset({ ...draft, draft: false }, "0.27.2", source, asset), false)
  assert.equal(audited0272QuarantinedAsset(draft, "0.27.3", source, asset), false)
})

test("historical version override requires explicit dispatch from v0.27.3", () => {
  assert.equal(recoveryVersion({ GITHUB_EVENT_NAME: "workflow_dispatch", RECOVERY_VERSION_OVERRIDE: "0.27.2" }, "0.27.3"), "0.27.2")
  assert.equal(recoveryVersion({ GITHUB_EVENT_NAME: "schedule" }, "0.27.3"), "0.27.3")
  assert.throws(() => recoveryVersion({ GITHUB_EVENT_NAME: "schedule", RECOVERY_VERSION_OVERRIDE: "0.27.2" }, "0.27.3"))
  assert.throws(() => recoveryVersion({ GITHUB_EVENT_NAME: "workflow_dispatch", RECOVERY_VERSION_OVERRIDE: "0.27.2" }, "0.27.4"))
  assert.throws(() => recoveryVersion({ GITHUB_EVENT_NAME: "workflow_dispatch", RECOVERY_VERSION_OVERRIDE: "0.27.1" }, "0.27.3"))
})

test("dispatch recovery is restricted to v0.27.2 and verified as recovery, never native", () => {
  const workflow = readFileSync(new URL("../workflows/release-recover.yml", import.meta.url), "utf8")
  const recover = readFileSync(new URL("./release-recover.mjs", import.meta.url), "utf8")
  const guard = readFileSync(new URL("./release-publication-guard.mjs", import.meta.url), "utf8")
  assert.match(workflow, /RECOVERY_VERSION_OVERRIDE: \$\{\{ inputs\.recovery_version \}\}/)
  assert.match(recover, /return \{ \.\.\.incident, recovery_only: true \}/)
  assert.match(recover, /version === "0\.27\.2" && run\.event === "workflow_dispatch"/)
  assert.match(recover, /make_latest: version === "0\.27\.2" \? "false" : "true"/)
  assert.match(guard, /version === "0\.27\.2" && run\.event === "workflow_dispatch"/)
})
