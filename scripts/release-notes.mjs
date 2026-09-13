const required = ["Changed", "Fixed", "Security", "Migration"]

const fallback = new Map([
  ["Changed", "- No notable changes in this release."],
  ["Fixed", "- No fixes specific to this release."],
  ["Security", "- No security-specific changes in this release."],
  ["Migration", "- No migration steps are required for this release."],
])

export const requiredReleaseSections = Object.freeze([...required])

export function releaseSection(changelog, version) {
  if (!/^\d+\.\d+\.\d+$/.test(version)) throw new Error("release notes require stable SemVer")
  const normalized = String(changelog).replace(/\r\n/g, "\n")
  const start = normalized.indexOf(`## ${version} - `)
  if (start < 0) throw new Error(`changelog is missing release ${version}`)
  const end = normalized.indexOf("\n## ", start + 1)
  return normalized.slice(start, end < 0 ? undefined : end).trim()
}

export function canonicalReleaseNotes(section) {
  const normalized = String(section).replace(/\r\n/g, "\n").trim()
  const lines = normalized.split("\n")
  if (!/^## \d+\.\d+\.\d+ - \d{4}-\d{2}-\d{2}$/.test(lines[0] || "")) throw new Error("release notes heading is malformed")

  const sections = new Map()
  const order = []
  let current = null
  for (const line of lines.slice(1)) {
    const heading = line.match(/^### ([A-Za-z][A-Za-z ]*)$/)
    if (heading) {
      current = heading[1]
      if (sections.has(current)) throw new Error(`duplicate release notes section ${current}`)
      sections.set(current, [])
      order.push(current)
      continue
    }
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
