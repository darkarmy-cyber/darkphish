import { execFileSync } from "node:child_process"
import { readdirSync, readFileSync, unlinkSync, writeFileSync } from "node:fs"
import { join } from "node:path"
import { fileURLToPath } from "node:url"

const rootURL = new URL("../", import.meta.url)
const root = fileURLToPath(rootURL)
const pathAtRoot = (value) => join(root, value)
const versionPath = pathAtRoot("VERSION")
const changelogPath = pathAtRoot("CHANGELOG.md")
const changesPath = pathAtRoot("changes/")
const categories = new Set(["Added", "Changed", "Deprecated", "Removed", "Fixed", "Security", "Breaking", "Migration"])
const semver = /^\d+\.\d+\.\d+$/

function version() {
  const value = readFileSync(versionPath, "utf8").trim()
  if (!semver.test(value)) throw new Error(`VERSION must be SemVer, got ${JSON.stringify(value)}`)
  return value
}

function nextPatch(value) {
  const [major, minor, patch] = value.split(".").map(Number)
  return `${major}.${minor}.${patch + 1}`
}

function nextMinor(value) {
  const [major, minor] = value.split(".").map(Number)
  return `${major}.${minor + 1}.0`
}

function fragments() {
  return readdirSync(changesPath, { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name !== "README.md" && entry.name.endsWith(".md"))
    .map((entry) => {
      if (!/^[a-z0-9][a-z0-9-]*\.md$/.test(entry.name)) throw new Error(`invalid fragment filename: ${entry.name}`)
      const body = readFileSync(join(changesPath, entry.name), "utf8").replace(/\r\n/g, "\n")
      const match = body.match(/^---\ncategory: ([A-Za-z]+)\nversion: (\d+\.\d+\.\d+)\n---\n([\s\S]+)$/)
      if (!match || !categories.has(match[1])) throw new Error(`invalid fragment schema: ${entry.name}`)
      const bullets = match[3].trim().split("\n").filter(Boolean)
      if (bullets.length === 0 || bullets.some((line) => !line.startsWith("- "))) throw new Error(`fragment must contain Markdown bullets: ${entry.name}`)
      return { name: entry.name, category: match[1], version: match[2], bullets }
    })
}

function changedFiles(base) {
  return execFileSync("git", ["diff", "--name-only", `${base}...HEAD`], { cwd: root, encoding: "utf8" })
    .split(/\r?\n/).filter(Boolean)
}

function validate(requirePRFragment = false, base = "") {
  const current = version()
  const values = fragments()
  const targets = new Set(values.map((fragment) => fragment.version))
  if (targets.size > 1) throw new Error("all changelog fragments must target the same release")
  const allowedTargets = new Set([current, nextPatch(current), nextMinor(current)])
  for (const fragment of values) {
    if (!allowedTargets.has(fragment.version)) {
      throw new Error(`${fragment.name} targets ${fragment.version}; expected ${current}, next patch ${nextPatch(current)}, or next minor ${nextMinor(current)}`)
    }
  }
  if (requirePRFragment) {
    const files = changedFiles(base)
    const runtimeChange = files.some((file) => !file.startsWith("changes/") && !file.startsWith("docs/") && !file.endsWith(".md") && !file.startsWith(".github/"))
    const fragmentChange = files.some((file) => /^changes\/(?!README\.md$).+\.md$/.test(file))
    if (runtimeChange && !fragmentChange) throw new Error("runtime changes require a changelog fragment in changes/")
  }
  return values
}

function aggregate() {
  const values = validate()
  if (values.length === 0) throw new Error("no changelog fragments to aggregate")
  const current = values[0].version
  if (version() !== current) writeFileSync(versionPath, `${current}\n`)
  let changelog = readFileSync(changelogPath, "utf8").replace(/\r\n/g, "\n")
  const heading = `## ${current}`
  if (!changelog.includes(heading)) {
    const insertion = [`${heading} - ${new Date().toISOString().slice(0, 10)}`, ""]
    for (const category of categories) {
      const bullets = values.filter((item) => item.category === category).flatMap((item) => item.bullets)
      if (bullets.length) insertion.push(`### ${category}`, "", ...bullets, "")
    }
    const titleEnd = changelog.indexOf("\n\n", changelog.indexOf("# Changelog"))
    changelog = changelog.slice(0, titleEnd + 2) + insertion.join("\n") + "\n" + changelog.slice(titleEnd + 2)
  } else {
    const start = changelog.indexOf(heading)
    const next = changelog.indexOf("\n## ", start + heading.length)
    const end = next < 0 ? changelog.length : next
    let section = changelog.slice(start, end).trimEnd()
    for (const category of categories) {
      const bullets = values.filter((item) => item.category === category).flatMap((item) => item.bullets)
        .filter((bullet) => !section.split("\n").includes(bullet))
      if (!bullets.length) continue
      const categoryHeading = `### ${category}\n`
      const categoryStart = section.indexOf(categoryHeading)
      if (categoryStart < 0) section += `\n\n${categoryHeading}\n${bullets.join("\n")}`
      else {
        const nextCategory = section.indexOf("\n### ", categoryStart + categoryHeading.length)
        const categoryEnd = nextCategory < 0 ? section.length : nextCategory
        section = section.slice(0, categoryEnd).trimEnd() + "\n" + bullets.join("\n") + "\n" + section.slice(categoryEnd)
      }
    }
    changelog = changelog.slice(0, start) + section + "\n" + (next < 0 ? "" : "\n" + changelog.slice(next + 1))
  }
  writeFileSync(changelogPath, changelog)
  for (const item of values) unlinkSync(join(changesPath, item.name))
}

const command = process.argv[2] || "validate"
try {
  if (command === "target") {
    const values = validate()
    process.stdout.write(`${values[0]?.version || version()}\n`)
  } else if (command === "validate") {
    const baseIndex = process.argv.indexOf("--base")
    validate(baseIndex !== -1, baseIndex !== -1 ? process.argv[baseIndex + 1] : "")
  } else if (command === "prepare" || command === "aggregate") {
    aggregate()
  } else {
    throw new Error(`unknown command ${command}`)
  }
  if (command !== "target") process.stdout.write(`changelog ${command} succeeded for ${version()}\n`)
} catch (error) {
  process.stderr.write(`${error.message}\n`)
  process.exitCode = 1
}
