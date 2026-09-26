import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { pathToFileURL } from "node:url"

// These sentinels intentionally fail closed if removed or their fixture syntax
// changes. fast-uri is transitive: package.json alone cannot supply its version.
export function smokeRepresentatives({ goMod, goSum, pnpmLock }) {
  const goVersions = [...goMod.matchAll(/^\s*github\.com\/gorilla\/mux (v[^\s]+)\s*$/gm)]
  assert.equal(goVersions.length, 1, "Expected one pinned Go sentinel")
  const goVersion = goVersions[0][1]
  assert.ok(goSum.split(/\r?\n/).some(line => line.startsWith(`github.com/gorilla/mux ${goVersion} h1:`)), "Go sentinel must have a module checksum")
  const packageSection = pnpmLock.replace(/\r\n/g, "\n").split(/^packages:\n/m)[1]?.split(/^snapshots:\n/m)[0]
  assert.ok(packageSection, "Expected pnpm packages section")
  const npmVersions = [...packageSection.matchAll(/^  fast-uri@(\d+\.\d+\.\d+):$/gm)]
  assert.equal(npmVersions.length, 1, "Expected one pinned pnpm transitive sentinel")
  return [
    { name: "github.com/gorilla/mux", version: goVersion, purl: `pkg:golang/github.com/gorilla/mux@${goVersion}` },
    { name: "fast-uri", version: npmVersions[0][1], purl: `pkg:npm/fast-uri@${npmVersions[0][1]}` },
  ]
}

export function validateSmokeSBOM(sbom, fixtures) {
  assert.match(sbom.spdxVersion, /^SPDX-2\./)
  assert.ok(Array.isArray(sbom.packages) && sbom.packages.length > 0)
  assert.ok(sbom.packages.every(item => typeof item?.name === "string" && item.SPDXID))
  for (const expected of smokeRepresentatives(fixtures)) {
    assert.ok(sbom.packages.some(item => item.name === expected.name && item.versionInfo === expected.version &&
      item.externalRefs?.some(ref => ref.referenceType === "purl" && typeof ref.referenceLocator === "string" &&
        ref.referenceLocator.split("?")[0] === expected.purl)), `Missing pinned ecosystem sentinel ${expected.purl}`)
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const sbom = JSON.parse(readFileSync(process.env.SBOM_FILE, "utf8"))
  validateSmokeSBOM(sbom, {
    goMod: readFileSync("go.mod", "utf8"), goSum: readFileSync("go.sum", "utf8"), pnpmLock: readFileSync("pnpm-lock.yaml", "utf8"),
  })
  console.log(`Validated SPDX with ${sbom.packages.length} packages and pinned Go/pnpm sentinels; no upload enabled.`)
}
