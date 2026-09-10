import assert from "node:assert/strict"
import test from "node:test"
import { execFileSync, spawnSync } from "node:child_process"
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"

test("release recovery aggregates additional 0.3 fragments without losing history or duplicating bullets", () => {
  const root = mkdtempSync(join(tmpdir(), "darkphish-changelog-test-"))
  try {
    mkdirSync(join(root, "scripts"))
    mkdirSync(join(root, "changes"))
    copyFileSync(new URL("changelog.mjs", import.meta.url), join(root, "scripts/changelog.mjs"))
    writeFileSync(join(root, "VERSION"), "0.3.0\n")
    const history = "## 0.2.0 - 2026-09-04\n\n### Fixed\n\n- historical fix\n"
    writeFileSync(join(root, "CHANGELOG.md"), "# Changelog\n\n## 0.3.0 - 2026-09-05\n\n### Fixed\n\n- existing fix\n\n" + history)
    writeFileSync(join(root, "changes/fix.md"), "---\ncategory: Fixed\nversion: 0.3.0\n---\n- existing fix\n- recovery fix\n")
    writeFileSync(join(root, "changes/security.md"), "---\ncategory: Security\nversion: 0.3.0\n---\n- secure automation\n")
    execFileSync(process.execPath, [join(root, "scripts/changelog.mjs"), "prepare"])
    const result = readFileSync(join(root, "CHANGELOG.md"), "utf8")
    assert.equal(result.split("- existing fix").length, 2)
    assert.match(result, /### Fixed\n\n- existing fix\n- recovery fix/)
    assert.match(result, /### Security\n\n- secure automation/)
    assert.ok(result.endsWith(history))
    assert.equal(readFileSync(join(root, "VERSION"), "utf8"), "0.3.0\n")
    execFileSync(process.execPath, [join(root, "scripts/changelog.mjs"), "validate"])
  } finally { rmSync(root, { recursive: true, force: true }) }
})

test("release metadata accepts current, next patch, and next minor targets", () => {
  for (const [current, target] of [["0.3.0", "0.4.0"], ["0.3.0", "0.3.1"], ["0.3.1", "0.3.2"], ["0.3.1", "0.3.1"], ["0.3.0", "0.3.0"]]) {
    const root = mkdtempSync(join(tmpdir(), "darkphish-target-test-"))
    try {
      mkdirSync(join(root, "scripts"))
      mkdirSync(join(root, "changes"))
      copyFileSync(new URL("changelog.mjs", import.meta.url), join(root, "scripts/changelog.mjs"))
      writeFileSync(join(root, "VERSION"), `${current}\n`)
      writeFileSync(join(root, "CHANGELOG.md"), "# Changelog\n\n## 0.2.0 - 2026-09-04\n")
      writeFileSync(join(root, "changes/fix.md"), `---\ncategory: Fixed\nversion: ${target}\n---\n- release fix\n`)
      const selected = execFileSync(process.execPath, [join(root, "scripts/changelog.mjs"), "target"], { encoding: "utf8" }).trim()
      assert.equal(selected, target)
      assert.equal(readFileSync(join(root, "VERSION"), "utf8"), `${current}\n`)
      execFileSync(process.execPath, [join(root, "scripts/changelog.mjs"), "prepare"])
      assert.equal(readFileSync(join(root, "VERSION"), "utf8"), `${target}\n`)
    } finally { rmSync(root, { recursive: true, force: true }) }
  }
})

test("release metadata permits concurrent patch and next-minor fragments and consumes only the selected patch", () => {
  const root = mkdtempSync(join(tmpdir(), "darkphish-mixed-target-test-"))
  try {
    mkdirSync(join(root, "scripts"))
    mkdirSync(join(root, "changes"))
    copyFileSync(new URL("changelog.mjs", import.meta.url), join(root, "scripts/changelog.mjs"))
    writeFileSync(join(root, "VERSION"), "0.7.0\n")
    writeFileSync(join(root, "CHANGELOG.md"), "# Changelog\n\n## 0.7.0 - 2026-09-07\n")
    writeFileSync(join(root, "changes/hotfix.md"), "---\ncategory: Fixed\nversion: 0.7.1\n---\n- patch fix\n")
    writeFileSync(join(root, "changes/future.md"), "---\ncategory: Security\nversion: 0.8.0\n---\n- future security change\n")

    execFileSync(process.execPath, [join(root, "scripts/changelog.mjs"), "validate"])
    const selected = execFileSync(process.execPath, [join(root, "scripts/changelog.mjs"), "target"], { encoding: "utf8" }).trim()
    assert.equal(selected, "0.7.1")

    execFileSync(process.execPath, [join(root, "scripts/changelog.mjs"), "prepare"])
    assert.equal(readFileSync(join(root, "VERSION"), "utf8"), "0.7.1\n")
    const changelog = readFileSync(join(root, "CHANGELOG.md"), "utf8")
    assert.match(changelog, /## 0\.7\.1[^\n]*\n\n### Fixed\n\n- patch fix/)
    assert.doesNotMatch(changelog, /future security change/)
    assert.deepEqual(readdirSync(join(root, "changes")).sort(), ["future.md"])
  } finally { rmSync(root, { recursive: true, force: true }) }
})

test("release metadata rejects skipped patch and other unsupported targets", () => {
  for (const target of ["0.3.2", "0.5.0", "1.0.0"]) {
    const root = mkdtempSync(join(tmpdir(), "darkphish-invalid-target-test-"))
    try {
      mkdirSync(join(root, "scripts"))
      mkdirSync(join(root, "changes"))
      copyFileSync(new URL("changelog.mjs", import.meta.url), join(root, "scripts/changelog.mjs"))
      writeFileSync(join(root, "VERSION"), "0.3.0\n")
      writeFileSync(join(root, "CHANGELOG.md"), "# Changelog\n")
      writeFileSync(join(root, "changes/fix.md"), `---\ncategory: Fixed\nversion: ${target}\n---\n- invalid release target\n`)
      const result = spawnSync(process.execPath, [join(root, "scripts/changelog.mjs"), "validate"], { encoding: "utf8" })
      assert.notEqual(result.status, 0)
      assert.match(result.stderr, /expected 0\.3\.0, next patch 0\.3\.1, or next minor 0\.4\.0/)
    } finally { rmSync(root, { recursive: true, force: true }) }
  }
})
