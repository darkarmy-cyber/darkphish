import assert from "node:assert/strict"
import test from "node:test"
import { execFileSync } from "node:child_process"
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs"
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

test("release metadata uses validated fragment targets before aggregation", () => {
  for (const [current, target] of [["0.3.0", "0.4.0"], ["0.3.1", "0.3.1"], ["0.3.0", "0.3.0"]]) {
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
