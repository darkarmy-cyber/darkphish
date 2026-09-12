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
  assert.match(workflow, /security-events: read/)
  assert.doesNotMatch(workflow, /security-events: write/)
  assert.match(refresh, /event_type: "generated-release-codeql"/)
  assert.doesNotMatch(refresh, /codeql-ondemand\.yml\/dispatches/)
  assert.match(validator, /generatedPath/)
  assert.match(validator, /files\.some\(\(file\) => !generatedPath/)
})

test("generated-release CodeQL reconciles local evidence only against the unchanged protected baseline", () => {
  const workflow = read("../.github/workflows/codeql-ondemand.yml")
  const validator = read("./release-codeql-target.mjs")

  assert.match(workflow, /ruleId:/)
  assert.match(workflow, /artifactLocation\?\.uri/)
  assert.match(workflow, /region\?\.startLine/)
  assert.match(workflow, /Inspect exact-head local CodeQL results/)
  assert.match(workflow, /Revalidate generated target and protected CodeQL baseline/)
  assert.match(workflow, /run: node scripts\/release-codeql-target\.mjs/g)
  assert.doesNotMatch(workflow, /filter\([^\n]*(?:diagnostic|warning|note|security)/i)
  assert.doesNotMatch(workflow, /findings\.length\s*!==?\s*0[\s\S]{0,300}throw/)
  assert.match(validator, /verifyCodeQLBaseline/)
  assert.match(validator, /await verifyCodeQLBaseline\(repo, pr\.base\.sha\)/)
  assert.match(validator, /finalPR\?\.base\?\.sha !== pr\.base\.sha/)
  assert.match(validator, /finalRef\?\.object\?\.sha !== sha/)
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
