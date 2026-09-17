import { readFileSync, readdirSync } from "node:fs"
import { spawnSync } from "node:child_process"
import { pathToFileURL } from "node:url"

export const protectedWorkflows = Object.freeze([
  "automerge.yml", "dependabot-automerge.yml", "release.yml",
  "release-recover.yml", "release-reconcile.yml", "release-resume.yml",
  "release-title-repair.yml",
])

// Temporary, deliberately narrow adapter for actionlint v1.7.12. Validate the
// entire literal policy before projecting ONLY its queue line out of lint input.
// No diagnostics are filtered, source files are not rewritten, and line numbers
// plus the original filename/project context are preserved. Unsupported layouts
// fail closed; they must not silently acquire a different concurrency policy.
export function lintInput(name, source) {
  if (!protectedWorkflows.includes(name)) return source
  // The PR-target merger admits only its authorized job to the shared queue.
  const indent = name === "automerge.yml" ? "    " : ""
  const policy = new RegExp(`^${indent}concurrency:\\r?\\n${indent}  group: protected-main-mutation\\r?\\n${indent}  cancel-in-progress: false\\r?\\n${indent}  queue: max\\r?\\n(?=\\r?\\n|${indent}[^\\s#])`, "gm")
  const matches = [...source.matchAll(policy)]
  if (matches.length !== 1) throw new Error(`${name}: expected exactly one literal protected mutation queue policy`)
  return source.replace(policy, block => block.replace(`${indent}  queue: max`, ""))
}

export function lintWorkflows(directory = ".github/workflows", executable = "actionlint") {
  const names = readdirSync(directory).filter(name => /\.ya?ml$/.test(name)).sort()
  for (const name of protectedWorkflows) {
    if (!names.includes(name)) throw new Error(`Missing protected workflow ${name}`)
  }
  let failed = false
  for (const name of names) {
    const path = `${directory}/${name}`
    const input = lintInput(name, readFileSync(path, "utf8"))
    const result = spawnSync(executable, ["-stdin-filename", path, "-"], { input, encoding: "utf8", stdio: ["pipe", "inherit", "inherit"] })
    if (result.error) throw result.error
    if (result.status !== 0) failed = true
  }
  if (failed) throw new Error("Workflow lint failed")
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try { lintWorkflows() } catch (error) { console.error(error.message); process.exitCode = 1 }
}
