import test from "node:test"
import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { normalizationHold } from "./release-normalization-hold.mjs"

const source = readFileSync(new URL("./release-normalization-hold.mjs", import.meta.url), "utf8")
const nativeWorkflow = readFileSync(new URL("../.github/workflows/release.yml", import.meta.url), "utf8")
const recoveryWorkflow = readFileSync(new URL("../.github/workflows/release-recover.yml", import.meta.url), "utf8")
const reconcileWorkflow = readFileSync(new URL("../.github/workflows/release-reconcile.yml", import.meta.url), "utf8")

test("reviewed v0.7.1 resumption removes the temporary normalization hold", () => {
  assert.equal(normalizationHold(), null)
  assert.match(reconcileWorkflow, /if: hashFiles\('\.github\/release-normalization-hold\.json'\) != ''/)
})

test("normalization hold fails closed on schema, fields and source identity", () => {
  assert.match(source, /unexpected fields/)
  assert.match(source, /schema is unsupported/)
  assert.match(source, /source SHA is invalid/)
  assert.match(source, /does not match current VERSION/)
  assert.match(source, /versionTag\(value\.version\)/)
})

test("all release mutation workflows share one serialization group", () => {
  for (const workflow of [nativeWorkflow, recoveryWorkflow, reconcileWorkflow]) {
    assert.match(workflow, /concurrency:\n\s+group: protected-main-mutation\n\s+cancel-in-progress: false/)
  }
})

test("publication workflows stop before mutation while normalization hold is active", () => {
  assert.match(nativeWorkflow, /ready: \$\{\{ steps\.hold\.outputs\.active == 'true' && 'false' \|\| steps\.version\.outputs\.ready \}\}/)
  assert.match(nativeWorkflow, /id: version\n\s+if: steps\.hold\.outputs\.active != 'true'/)
  assert.match(recoveryWorkflow, /ready: \$\{\{ steps\.hold\.outputs\.active == 'true' && 'false'/)
  assert.match(recoveryWorkflow, /id: recovery\n\s+if: steps\.hold\.outputs\.active != 'true'/)
})

test("reconciliation withdraws trusted publication before repairing the historical draft", () => {
  const hold = reconcileWorkflow.indexOf("Enforce trusted normalization hold")
  const repair = reconcileWorkflow.indexOf("Repair trusted detached draft tag")
  const reconcile = reconcileWorkflow.indexOf("Reconcile trusted release metadata")
  assert.ok(hold > 0 && hold < repair && repair < reconcile)
  assert.match(reconcileWorkflow, /release-publication-guard\.mjs hold/)
})
