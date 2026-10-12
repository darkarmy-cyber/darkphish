// Historical native runs keep their literal generated step names. Admit only
// the three reviewed action generations; floating refs are never evidence.
const trustedNames = new Set([
  "Run anchore/sbom-action@f8bdd1d8ac5e901a77a92f111440fdb1b593736b", // v0.20.6, Node20
  "Run anchore/sbom-action@3ad7283483fc7af8ff2b4ea19663c2d5ca935e26", // v0.24.2, Node24
  "Run anchore/sbom-action@66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6c", // v0.24.3, Node24
])

export function successfulTrustedSBOMStep(job) {
  if (!Array.isArray(job?.steps)) return false
  const generators = job.steps.filter((step) => typeof step?.name === "string" && step.name.startsWith("Run anchore/sbom-action@"))
  return generators.length === 1 && trustedNames.has(generators[0].name) &&
    generators[0].status === "completed" && generators[0].conclusion === "success"
}
