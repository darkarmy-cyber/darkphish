import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"

const releaseWorkflow = readFileSync(new URL("../.github/workflows/release.yml", import.meta.url), "utf8")

test("native release metadata fetches tags before changelog validation", () => {
  const metadata = releaseWorkflow.match(/\n  metadata:\n[\s\S]*?\n  verify:/)?.[0] || ""
  assert.match(metadata, /uses: actions\/checkout@v7[\s\S]*?ref: main[\s\S]*?fetch-depth: 0/)
  const checkout = metadata.indexOf("uses: actions/checkout@v7")
  const validate = metadata.indexOf("node scripts/changelog.mjs validate")
  assert.ok(checkout >= 0 && validate > checkout)
})
