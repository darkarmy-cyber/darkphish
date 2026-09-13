import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"

test("release recovery reproduces historical changelog date from trusted PR metadata", () => {
  const source = readFileSync(new URL("./release-prepare.mjs", import.meta.url), "utf8")
  assert.match(source, /function trustedReleaseDate\(pr\)/)
  assert.match(source, /pr\?\.created_at/)
  assert.match(source, /existing release PR is missing a valid server creation date/)
  assert.match(source, /const existingPR = prs\[0\]/)
  assert.match(source, /const releaseDate = trustedReleaseDate\(existingPR\)/)
  assert.match(source, /const heading = `## \$\{version\} - `/)
  assert.match(source, /lines\[headingIndex\] = `\$\{heading\}\$\{releaseDate\}`/)
  assert.ok(source.indexOf("lines[headingIndex]") < source.indexOf("const expected = execFileSync"))
  assert.doesNotMatch(source, /oldSHA.*created_at|git\([^\n]+--format=%[ac]I/)
})
