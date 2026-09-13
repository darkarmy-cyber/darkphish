const required = ["Changed", "Fixed", "Security", "Migration"]
const stableSemver = "(0|[1-9]\\d*)\\.(0|[1-9]\\d*)\\.(0|[1-9]\\d*)"

const fallback = new Map([
  ["Changed", "- No notable changes in this release."],
  ["Fixed", "- No fixes specific to this release."],
  ["Security", "- No security-specific changes in this release."],
  ["Migration", "- No migration steps are required for this release."],
])

export const requiredReleaseSections = Object.freeze([...required])

export function releaseSection(changelog, version) {
  if (!new RegExp(`^${stableSemver}$`).test(version)) throw new Error("release notes require stable SemVer")
  const normalized = String(changelog).replace(/\r\n/g, "\n")
  const heading = new RegExp(`^## ${version.replace(/\./g, "\\.")} - \\d{4}-\\d{2}-\\d{2}$`, "m")
  const match = heading.exec(normalized)
  if (!match) throw new Error(`changelog is missing release ${version}`)
  const start = match.index
  const nextHeading = /^## (0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*) - \d{4}-\d{2}-\d{2}$/gm
  nextHeading.lastIndex = start + match[0].length
  const next = nextHeading.exec(normalized)
  return normalized.slice(start, next ? next.index : undefined).trim()
}

export function canonicalReleaseNotes(section) {
  const normalized = String(section).replace(/\r\n/g, "\n").trim()
  const lines = normalized.split("\n")
  if (!new RegExp(`^## ${stableSemver} - \\d{4}-\\d{2}-\\d{2}$`).test(lines[0] || "")) throw new Error("release notes heading is malformed")

  const sections = new Map()
  const order = []
  let current = null
  for (const line of lines.slice(1)) {
    const heading = line.match(/^### ([A-Za-z](?:[A-Za-z ]*[A-Za-z])?)$/)
    if (heading) {
      current = heading[1]
      if (sections.has(current)) throw new Error(`duplicate release notes section ${current}`)
      sections.set(current, [])
      order.push(current)
      continue
    }
    if (/^###\s/.test(line)) throw new Error("release notes section heading is malformed")
    if (!current) {
      if (line.trim()) throw new Error("release notes contain content outside a section")
      continue
    }
    sections.get(current).push(line)
  }

  const rendered = [lines[0], ""]
  const append = (name, body) => {
    const text = body.join("\n").trim()
    rendered.push(`### ${name}`, "", text || fallback.get(name) || "- No notable changes in this release.", "")
  }
  for (const name of required) append(name, sections.get(name) || [])
  for (const name of order) if (!required.includes(name)) append(name, sections.get(name) || [])
  return rendered.join("\n").trim()
}

export function releaseBody(changelog, version, source) {
  if (!/^[a-f0-9]{40}$/.test(source || "")) throw new Error("release source SHA is malformed")
  const notes = canonicalReleaseNotes(releaseSection(changelog, version))
  return `${notes}\n\nSource commit: ${source}\n\n<!-- darkphish-release-source:${source} -->\n\nNative binaries, SHA-256 checksums and SPDX SBOM are attached.`
}
