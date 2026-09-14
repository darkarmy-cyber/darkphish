const required = ["Changed", "Fixed", "Security", "Migration"]
const stableSemver = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/
const releaseHeading = /^## ((?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)) - \d{4}-\d{2}-\d{2}$/gm
const releaseHeadingLine = /^## (0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*) - \d{4}-\d{2}-\d{2}$/
const legacyTrailer = "All notable changes to Darkphish are documented here. The project follows\nSemantic Versioning while the public API and schema are still pre-1.0."

const fallback = new Map([
  ["Changed", "- No notable changes in this release."],
  ["Fixed", "- No fixes specific to this release."],
  ["Security", "- No security-specific changes in this release."],
  ["Migration", "- No migration steps are required for this release."],
])

const categoryMap = new Map([
  ["Changed", "Changed"],
  ["Added", "Changed"],
  ["Deprecated", "Changed"],
  ["Fixed", "Fixed"],
  ["Security", "Security"],
  ["Migration", "Migration"],
  ["Breaking", "Migration"],
])

export const requiredReleaseSections = Object.freeze([...required])

export function releaseSection(changelog, version) {
  if (!stableSemver.test(version)) throw new Error("release notes require stable SemVer")
  const normalized = String(changelog).replace(/\r\n/g, "\n")
  const headings = [...normalized.matchAll(releaseHeading)]
  const index = headings.findIndex((match) => match[1] === version)
  if (index < 0) throw new Error(`changelog is missing release ${version}`)
  const match = headings[index]
  const next = headings[index + 1]
  let section = normalized.slice(match.index, next ? next.index : undefined).trim()
  if (section.endsWith(legacyTrailer)) section = section.slice(0, -legacyTrailer.length).trim()
  return section
}

export function canonicalReleaseNotes(section) {
  const normalized = String(section).replace(/\r\n/g, "\n").trim()
  const lines = normalized.split("\n")
  if (!releaseHeadingLine.test(lines[0] || "")) throw new Error("release notes heading is malformed")

  const rawSections = new Map()
  let current = null
  for (const line of lines.slice(1)) {
    const heading = line.match(/^### ([A-Za-z](?:[A-Za-z ]*[A-Za-z])?)$/)
    if (heading) {
      current = heading[1]
      if (!categoryMap.has(current)) throw new Error(`unsupported release notes section ${current}`)
      if (rawSections.has(current)) throw new Error(`duplicate release notes section ${current}`)
      rawSections.set(current, [])
      continue
    }
    if (/^###\s/.test(line)) throw new Error("release notes section heading is malformed")
    if (!current) {
      if (line.trim()) throw new Error("release notes contain content outside a section")
      continue
    }
    rawSections.get(current).push(line)
  }

  const canonical = new Map(required.map((name) => [name, []]))
  for (const [name, body] of rawSections) {
    const target = categoryMap.get(name)
    const text = body.join("\n").trim()
    if (text) canonical.get(target).push(text)
  }

  const rendered = [lines[0], ""]
  for (const name of required) {
    const text = canonical.get(name).join("\n\n").trim() || fallback.get(name)
    rendered.push(`### ${name}`, "", text, "")
  }
  return rendered.join("\n").trim()
}

export function releaseBody(changelog, version, source) {
  if (!/^[a-f0-9]{40}$/.test(source || "")) throw new Error("release source SHA is malformed")
  const notes = canonicalReleaseNotes(releaseSection(changelog, version))
  return `${notes}\n\nSource commit: ${source}\n\n<!-- darkphish-release-source:${source} -->\n\nNative binaries, SHA-256 checksums and SPDX SBOM are attached.`
}
