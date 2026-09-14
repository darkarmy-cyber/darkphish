export function verifyReceiptBackedArtifacts({ version, source, assets, local, manifestText, receiptText }) {
  const tag = `v${version}`
  if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(version || "") || !/^[a-f0-9]{40}$/.test(source || "")) throw new Error("staged release identity is malformed")
  const receiptName = `darkphish-${tag}.release.json`
  const manifest = local.get("SHA256SUMS"), receiptFile = local.get(receiptName)
  if (!manifest || !receiptFile || typeof manifestText !== "string" || typeof receiptText !== "string") throw new Error("receipt-backed release evidence is incomplete")
  if (manifestText.length > 65536 || receiptText.length > 4096) throw new Error("receipt-backed release evidence is unexpectedly large")

  const expected = new Map()
  for (const line of manifestText.trim().split(/\r?\n/)) {
    const match = line.match(/^([a-f0-9]{64})  (darkphish-[A-Za-z0-9._-]+)$/)
    if (!match || expected.has(match[2])) throw new Error("receipt-backed checksum manifest is malformed")
    expected.set(match[2], match[1])
  }
  const payloads = assets.filter((asset) => asset.name !== "SHA256SUMS" && asset.name !== receiptName)
  if (expected.size !== payloads.length) throw new Error("receipt-backed checksum manifest is incomplete")
  for (const asset of payloads) {
    const downloaded = local.get(asset.name)
    if (!downloaded || expected.get(asset.name) !== downloaded.digest || asset.digest !== `sha256:${downloaded.digest}`) throw new Error("receipt-backed asset is not cryptographically bound to its manifest")
  }

  let receipt
  try { receipt = JSON.parse(receiptText) } catch { throw new Error("receipt-backed publication receipt is malformed") }
  const wanted = {
    schema: "darkphish-release-publication-receipt/v1",
    tag,
    source_sha: source,
    checksums_sha256: manifest.digest,
  }
  if (Object.keys(receipt).sort().join("\n") !== Object.keys(wanted).sort().join("\n") || Object.entries(wanted).some(([key, value]) => receipt[key] !== value) || !/^[a-f0-9]{64}$/.test(manifest.digest || "")) throw new Error("receipt-backed publication receipt does not prove the immutable source and artifact set")
  return true
}

export function recoveryJobsProveStagedArtifacts(jobs) {
  const successfulStep = (job, name) => job?.steps?.some((step) => step.name === name && step.status === "completed" && step.conclusion === "success")
  const metadata = jobs.find((job) => job.name === "metadata")
  const verify = jobs.find((job) => job.name === "verify")
  const smoke = jobs.find((job) => job.name === "audit-smoke")
  const publish = jobs.find((job) => job.name === "publish")
  const binaries = jobs.filter((job) => job.name?.startsWith("binaries ("))
  if (metadata?.conclusion !== "success" || verify?.conclusion !== "success" || smoke?.conclusion !== "success" || publish?.conclusion !== "success" || binaries.length !== 5 || binaries.some((job) => job.conclusion !== "success")) return false
  return successfulStep(metadata, "Verify historical release provenance and recovery source") &&
    successfulStep(publish, "Generate recovery checksums") &&
    successfulStep(publish, "Generate recovery publication receipt") &&
    successfulStep(publish, "Attest rebuilt recovery artifacts") &&
    successfulStep(publish, "Publish rebuilt verified recovery assets") &&
    successfulStep(publish, "Verify published recovery including receipt attestation")
}
