import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"

test("release recovery reproduces historical changelog date from trusted generated PR metadata", () => {
  const source = readFileSync(new URL("./release-prepare.mjs", import.meta.url), "utf8")
  assert.match(source, /function trustedGeneratedReleasePR\(repo, branch, version, pr\)/)
  assert.match(source, /pr\.state !== "open"/)
  assert.match(source, /pr\.draft !== false/)
  assert.match(source, /pr\.base\?\.ref !== "main"/)
  assert.match(source, /pr\.head\?\.ref !== branch/)
  assert.match(source, /pr\.head\?\.repo\?\.full_name !== repo/)
  assert.match(source, /pr\.title !== `release: Darkphish \$\{version\}`/)
  assert.match(source, /pr\.user\?\.login !== "github-actions\[bot\]"/)
  assert.match(source, /pr\.user\?\.id !== 41898282/)
  assert.match(source, /pr\.user\?\.type !== "Bot"/)
  assert.match(source, /function trustedReleaseDate\(pr\)/)
  assert.match(source, /pr\?\.created_at/)
  assert.match(source, /existing release PR is missing a valid server creation date/)
  assert.match(source, /const existingPR = trustedGeneratedReleasePR\(repo, branch, version, prs\[0\]\)/)
  assert.match(source, /const releaseDate = trustedReleaseDate\(existingPR\)/)
  assert.match(source, /const heading = `## \$\{version\} - `/)
  assert.match(source, /lines\[headingIndex\] = `\$\{heading\}\$\{releaseDate\}`/)
  assert.ok(source.indexOf("lines[headingIndex]") < source.indexOf("const expected = execFileSync"))
  assert.doesNotMatch(source, /oldSHA.*created_at|git\([^\n]+--format=%[ac]I/)
})
