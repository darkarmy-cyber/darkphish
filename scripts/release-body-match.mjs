export function comparableReleaseBody(value) {
  return typeof value === "string" ? value.replace(/\r\n/g, "\n") : value
}

export function matchesTrustedReleaseBody(actual, ...expected) {
  const comparable = comparableReleaseBody(actual)
  return expected.some((candidate) => comparable === candidate)
}
