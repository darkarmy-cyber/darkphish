import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"
import { successfulTrustedSBOMStep } from "./release-sbom-step.mjs"

const oldPin = "f8bdd1d8ac5e901a77a92f111440fdb1b593736b"
const newPin = "3ad7283483fc7af8ff2b4ea19663c2d5ca935e26"
const step = (pin, extra = {}) => ({ name: `Run anchore/sbom-action@${pin}`, status: "completed", conclusion: "success", ...extra })

for (const pin of [oldPin, newPin]) {
  test(`historical native recovery accepts exact successful generation ${pin}`, () => {
    assert.equal(successfulTrustedSBOMStep({ conclusion: "failure", steps: [{ name: "Set up job" }, step(pin), { name: "Generate checksums" }] }), true)
  })
  test(`failed, pending and incomplete generation is rejected ${pin}`, () => {
    for (const extra of [{ status: "in_progress" }, { status: "queued" }, { status: undefined }, { conclusion: "failure" }, { conclusion: "cancelled" }, { conclusion: "skipped" }, { conclusion: "neutral" }, { conclusion: undefined }]) {
      assert.equal(successfulTrustedSBOMStep({ steps: [step(pin, extra)] }), false)
    }
  })
}
test("unknown pins, tags, renamed and duplicate generators are not provenance", () => {
  for (const value of [undefined, {}, { steps: null }, { steps: [] }, { steps: [null] },
    { steps: [step("v0.24.2")] }, { steps: [step("main")] }, { steps: [step("a".repeat(40))] },
    { steps: [step(newPin.toUpperCase())] }, { steps: [step(newPin, { name: "Generate SBOM" })] },
    { steps: [step(`${newPin} extra`)] }, { steps: [step(newPin), step(newPin)] },
    { steps: [step(oldPin), step(newPin)] }, { steps: [step(newPin), step("main")] },
  ]) assert.equal(successfulTrustedSBOMStep(value), false)
})
test("recovery uses the bounded step predicate without removing other historical gates", () => {
  const source = readFileSync(new URL("./release-recover.mjs", import.meta.url), "utf8")
  assert.match(source, /!successfulTrustedSBOMStep\(publish\)/)
  for (const name of ["Run node scripts/release-publish.mjs metadata", "Run actions/download-artifact@v8", "Generate checksums", "Publish verified assets without overwriting an existing release"]) assert.ok(source.includes(name))
})
test("both publishers and read-only smoke use the reviewed pin with all uploads disabled", () => {
  for (const file of ["release.yml", "release-recover.yml", "sbom-smoke.yml"]) {
    const workflow = readFileSync(new URL(`../workflows/${file}`, import.meta.url), "utf8").replace(/\r\n/g, "\n")
    const matches = [...workflow.matchAll(/uses: anchore\/sbom-action@([^\s]+)/g)]
    assert.equal(matches.length, 1); assert.equal(matches[0][1], newPin)
    const inputs = workflow.slice(matches[0].index).split(/\n      - /)[0]
    for (const key of ["upload-artifact", "upload-release-assets", "dependency-snapshot"]) assert.match(inputs, new RegExp(`${key}: false`))
    assert.match(inputs, /format: spdx-json/)
  }
})
test("SBOM smoke has no write permission or privileged PR trigger", () => {
  const workflow = readFileSync(new URL("../workflows/sbom-smoke.yml", import.meta.url), "utf8")
  assert.match(workflow, /contents: read/)
  assert.doesNotMatch(workflow, /: write|write-all|pull_request_target|workflow_run|workflow_dispatch|secrets\./)
  assert.match(workflow, /persist-credentials: false/)
})
