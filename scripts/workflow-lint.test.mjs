import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync, readdirSync } from "node:fs"
import { spawnSync } from "node:child_process"
import { lintInput, protectedWorkflows } from "./workflow-lint.mjs"

const policy = "concurrency:\n  group: protected-main-mutation\n  cancel-in-progress: false\n  queue: max\n\n"
const fixture = `${policy}on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hello\n`

test("only the validated queue line is projected, with original line numbers", () => {
  for (const newline of ["\n", "\r\n"]) {
    const input = fixture.replaceAll("\n", newline)
    assert.equal(lintInput("release.yml", input), input.replace("  queue: max", ""))
    assert.equal(lintInput("ci.yml", input), input)
  }
})

test("missing, duplicate, dynamic or relaxed policies are refused", () => {
  for (const value of [
    fixture.replace("  queue: max\n", ""), fixture.replace("max", "single"),
    fixture.replace("false", "true"), fixture.replace("false", "${{ false }}"),
    fixture.replace("protected-main-mutation", "other"),
    fixture.replace("max", "${{ inputs.queue }}"), policy + fixture,
    fixture.replace("  queue: max", "  queue: 'max'"),
    fixture.replace("concurrency:", "concurrency: &lock"),
  ]) assert.throws(() => lintInput("release.yml", value), /queue policy/)
})

test("every shared-lock workflow opts into the same bounded queue", () => {
  const directory = new URL("../.github/workflows/", import.meta.url)
  const members = []
  for (const name of readdirSync(directory).filter(name => /\.ya?ml$/.test(name))) {
    const source = readFileSync(new URL(name, directory), "utf8")
    if (/group: protected-main-mutation/.test(source)) members.push(name)
    if (protectedWorkflows.includes(name)) assert.notEqual(lintInput(name, source), source)
  }
  assert.deepEqual(members.sort(), [...protectedWorkflows].sort())
})

test("PR-target trust gate precedes job-level queue admission", () => {
  const source = readFileSync(new URL("../.github/workflows/automerge.yml", import.meta.url), "utf8")
  assert.doesNotMatch(source, /^concurrency:/m)
  assert.match(source, /jobs:\n  enable:\n    if:/)
  const queue = source.indexOf("    concurrency:")
  for (const gate of ["github.ref == 'refs/heads/main'", "github.event.pull_request.base.ref == 'main'", "github.event.pull_request.head.repo.full_name == github.repository", "github.event.pull_request.author_association"])
    assert.ok(source.indexOf(gate) >= 0 && source.indexOf(gate) < queue)
  assert.equal(lintInput("automerge.yml", source), source.replace("      queue: max", ""))
  assert.throws(() => lintInput("automerge.yml", fixture), /queue policy/)
})

// Run in required CI after installing the pinned, unmodified upstream binary.
// Local Node-only test runs can omit it; CI explicitly requires these tests.
test("real actionlint still rejects syntax, expressions and duplicate mappings", { skip: !process.env.WORKFLOW_LINT_INTEGRATION }, () => {
  const run = source => spawnSync("actionlint", ["-stdin-filename", ".github/workflows/release.yml", "-"], {
    input: lintInput("release.yml", source), encoding: "utf8",
  })
  const valid = run(fixture)
  assert.ifError(valid.error); assert.equal(valid.status, 0, valid.stdout + valid.stderr)
  for (const bad of [
    fixture.replace("runs-on:", "runs-onn:"),
    fixture.replace("echo hello", "echo ${{ nonexistent.value }}"),
    fixture.replace("\non: push", "\nconcurrency: other\non: push"),
    fixture.replace("\non: push", "\n  group: other\non: push"),
    fixture.replace("\non: push", "\n  queue: max\non: push"),
    fixture.replace("\non: push", "\n  typo: true\non: push"),
    fixture.replace("runs-on: ubuntu-latest", "runs-on: ["),
  ]) {
    const result = run(bad)
    assert.ifError(result.error); assert.notEqual(result.status, 0, bad)
  }
})
