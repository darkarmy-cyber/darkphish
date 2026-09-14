const historicalPrivateFooterSource = "355d2881a4d5eea162d46a1c155998b1fb8c9f36"
const canonicalFooter = "Native binaries, SHA-256 checksums and SPDX SBOM are attached."
const historicalPrivateFooter = `${canonicalFooter} Artifact attestations require GitHub Enterprise Cloud for this private repository; see docs/REPOSITORY_ADMIN.md.`

export function comparableReleaseBody(value) {
  return typeof value === "string" ? value.replace(/\r\n/g, "\n") : value
}

function immutableHistoricalPublisherVariant(candidate) {
  if (typeof candidate !== "string") return null
  const sourceLine = `Source commit: ${historicalPrivateFooterSource}`
  const provenance = `<!-- darkphish-release-source:${historicalPrivateFooterSource} -->`
  if (!candidate.includes(sourceLine) || !candidate.includes(provenance) || !candidate.endsWith(canonicalFooter)) return null
  return `${candidate.slice(0, -canonicalFooter.length)}${historicalPrivateFooter}`
}

export function matchesTrustedReleaseBody(actual, ...expected) {
  const comparable = comparableReleaseBody(actual)
  return expected.some((candidate) => {
    if (comparable === candidate) return true
    const historical = immutableHistoricalPublisherVariant(candidate)
    return historical !== null && comparable === historical
  })
}
