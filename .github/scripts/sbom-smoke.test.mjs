import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"
import { smokeRepresentatives, validateSmokeSBOM } from "./sbom-smoke.mjs"

const fixtures = {
  goMod: "require (\n\tgithub.com/gorilla/mux v1.8.1\n)\n",
  goSum: "github.com/gorilla/mux v1.8.1 h1:fixture\n",
  pnpmLock: "lockfileVersion: '9.0'\npackages:\n  neo-async@2.6.2:\n    resolution: {}\nsnapshots:\n  neo-async@2.6.2: {}\n",
}
const packageFor = item => ({ name: item.name, versionInfo: item.version, SPDXID: `SPDXRef-${item.name}`, externalRefs: [{ referenceType: "purl", referenceLocator: item.purl }] })
const valid = () => ({ spdxVersion: "SPDX-2.3", packages: smokeRepresentatives(fixtures).map(packageFor) })

test("smoke accepts both pinned ecosystems including qualified Go purl", () => {
  const sbom = valid()
  sbom.packages[0].externalRefs[0].referenceLocator += "?goos=linux"
  validateSmokeSBOM(sbom, fixtures)
})
test("each missing ecosystem, wrong version or wrong purl fails despite nonempty packages", () => {
  for (const index of [0, 1]) {
    const missing = valid(); missing.packages.splice(index, 1)
    assert.throws(() => validateSmokeSBOM(missing, fixtures), /Missing pinned ecosystem/)
    const wrongVersion = valid(); wrongVersion.packages[index].versionInfo = "0.0.0"
    assert.throws(() => validateSmokeSBOM(wrongVersion, fixtures), /Missing pinned ecosystem/)
    const wrongPurl = valid(); wrongPurl.packages[index].externalRefs = []
    assert.throws(() => validateSmokeSBOM(wrongPurl, fixtures), /Missing pinned ecosystem/)
  }
})
test("removed, ambiguous or changed fixture syntax fails closed", () => {
  for (const change of [{ goMod: "" }, { goSum: "github.com/gorilla/mux v1.8.1/go.mod h1:fixture\n" },
    { pnpmLock: "snapshots:\n  neo-async@2.6.2: {}\n" },
    { pnpmLock: fixtures.pnpmLock.replace("  neo-async@2.6.2:\n", "  neo-async@2.6.2:\n  neo-async@2.6.3:\n") },
  ]) assert.throws(() => smokeRepresentatives({ ...fixtures, ...change }))
})
test("current repository fixtures supply both sentinels without hardcoded versions", () => {
  const source = file => readFileSync(new URL(`../../${file}`, import.meta.url), "utf8")
  const expected = smokeRepresentatives({ goMod: source("go.mod"), goSum: source("go.sum"), pnpmLock: source("pnpm-lock.yaml") })
  assert.equal(expected.length, 2)
  assert.equal(JSON.parse(source("package.json")).dependencies?.["neo-async"], undefined)
  assert.equal(JSON.parse(source("package.json")).devDependencies?.["neo-async"], undefined)
})
test("PR and main smoke triggers cover every copied manifest and validator", () => {
  const workflow = readFileSync(new URL("../workflows/sbom-smoke.yml", import.meta.url), "utf8").replace(/\r\n/g, "\n")
  for (const event of ["pull_request", "push"]) {
    const block = workflow.split(`  ${event}:\n`)[1]?.split(/\n(?:  [a-z_]+:|permissions:)/)[0]
    assert.ok(block)
    for (const path of ["go.mod", "go.sum", "package.json", "pnpm-lock.yaml", ".github/scripts/sbom-smoke*"]) assert.ok(block.includes(`- '${path}'`), `${event} must cover ${path}`)
  }
  assert.match(workflow, /node \.github\/scripts\/sbom-smoke\.mjs/)
})
