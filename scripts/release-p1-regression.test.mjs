import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"

const read = path => readFileSync(new URL(path, import.meta.url), "utf8")

test("generated-release CodeQL runs only from protected default-branch workflow code", () => {
  const workflow = read("../.github/workflows/codeql-ondemand.yml")
  const refresh = read("./check-refresh.mjs")
  const validator = read("./release-codeql-target.mjs")

  assert.doesNotMatch(workflow, /\bworkflow_dispatch\s*:/)
  assert.match(workflow, /repository_dispatch:/)
  assert.match(workflow, /types: \[generated-release-codeql\]/)
  assert.match(workflow, /github\.ref == 'refs\/heads\/main'/)
  assert.match(workflow, /actions\/checkout@v7[\s\S]*ref: main/)
  assert.match(workflow, /ref: \$\{\{ needs\.validate\.outputs\.target_sha \}\}/)
  assert.match(workflow, /upload: never/)
  assert.doesNotMatch(workflow, /security-events: write/)
  assert.match(refresh, /event_type: "generated-release-codeql"/)
  assert.doesNotMatch(refresh, /codeql-ondemand\.yml\/dispatches/)
  assert.match(validator, /generatedPath/)
  assert.match(validator, /files\.some\(\(file\) => !generatedPath/)
})

test("generated release refresh retries only head-SHA convergence while preserving immutable PR identity", () => {
  const refresh = read("./check-refresh.mjs")
  const shape = refresh.indexOf("pr?.draft !== false")
  const captureIdentity = refresh.indexOf("identity = releasePullIdentity(pr)")
  const rejectIdentityChange = refresh.indexOf("generated release PR identity changed during head convergence")
  const exactHead = refresh.indexOf("pr?.head?.sha === sha")
  const retry = refresh.indexOf("await sleep(releaseHeadConvergenceDelayMs)")
  const failure = refresh.indexOf("generated release PR head did not converge to the exact branch SHA")

  assert.ok(shape >= 0 && captureIdentity > shape && rejectIdentityChange > captureIdentity && exactHead > rejectIdentityChange && retry > exactHead && failure > retry)
  assert.match(refresh, /pr\?\.base\?\.ref !== "main"/)
  assert.match(refresh, /pr\?\.head\?\.ref !== branch/)
  assert.match(refresh, /pr\?\.head\?\.repo\?\.full_name !== repo/)
  assert.match(refresh, /pr\?\.title !== `release: Darkphish/)
  assert.match(refresh, /pr\?\.number === identity\.number/)
  assert.match(refresh, /pr\?\.id === identity\.id/)
  assert.match(refresh, /pr\?\.node_id === identity\.nodeID/)
  assert.match(refresh, /releaseHeadConvergenceAttempts = 6/)
})

test("generated release convergence pins the exact branch SHA before and after accepting the PR head", () => {
  const refresh = read("./check-refresh.mjs")
  const loop = refresh.indexOf("for (let attempt = 1; attempt <= releaseHeadConvergenceAttempts")
  const firstRefCheck = refresh.indexOf("await exactBranchSHA(repo, branch, sha, request)", loop)
  const exactHead = refresh.indexOf("pr?.head?.sha === sha", firstRefCheck)
  const secondRefCheck = refresh.indexOf("await exactBranchSHA(repo, branch, sha, request)", exactHead)
  const branchMoved = refresh.indexOf("generated release branch moved during pull-request head convergence")

  assert.ok(loop >= 0 && firstRefCheck > loop && exactHead > firstRefCheck && secondRefCheck > exactHead)
  assert.ok(branchMoved >= 0)
})

test("exact release check dispatches revalidate the branch ref immediately before mutable-ref dispatch", () => {
  const refresh = read("./check-refresh.mjs")
  const ciDispatch = refresh.indexOf("actions/workflows/ci.yml/dispatches")
  const codeqlDispatch = refresh.indexOf('event_type: "generated-release-codeql"')
  assert.ok(ciDispatch > 0 && codeqlDispatch > ciDispatch)
  assert.match(refresh.slice(Math.max(0, ciDispatch - 180), ciDispatch), /exactBranchSHA\(repo, branch, sha, request\)/)
  assert.match(refresh.slice(Math.max(0, codeqlDispatch - 220), codeqlDispatch), /exactBranchSHA\(repo, branch, sha, request\)/)
})

test("legacy engineering auto-merge is revoked before the publication freeze can return", () => {
  const lib = read("./release-lib.mjs")
  const cancel = lib.indexOf("if (pr.auto_merge)")
  const boundary = lib.indexOf("const enforceReleaseBoundary")
  const freeze = lib.indexOf("current VERSION is not fully published")
  assert.ok(cancel >= 0 && boundary > cancel && freeze > boundary)
  assert.match(lib.slice(cancel, boundary), /--disable-auto/)
})

test("Dependabot migration cleanup revokes old native queues before checking the release boundary", () => {
  const workflow = read("../.github/workflows/dependabot-automerge.yml")
  const disable = workflow.indexOf("--disable-auto")
  const boundary = workflow.indexOf("node scripts/require-green-main.mjs")
  const synchronous = workflow.indexOf("--match-head-commit")
  assert.ok(disable >= 0 && boundary > disable && synchronous > boundary)
  assert.match(workflow, /autoMergeRequest/)
  assert.doesNotMatch(workflow, /gh pr merge[^\n]*--auto(?:\s|$)/)
  assert.match(workflow, /group: protected-main-mutation/)
  assert.match(workflow, /cancel-in-progress: false/)
})
