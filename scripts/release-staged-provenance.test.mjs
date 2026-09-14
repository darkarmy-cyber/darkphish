import assert from "node:assert/strict"
import { createHash } from "node:crypto"
import test from "node:test"
import { recoveryJobsProveStagedArtifacts, verifyReceiptBackedArtifacts } from "./release-staged-provenance.mjs"

const version = "0.7.1"
const source = "73bf5948ed19cd918638453d478ef3f1cbe83d89"
const payloads = [
  ["darkphish-v0.7.1-linux-amd64.tar.gz", "1".repeat(64)],
  ["darkphish-v0.7.1.spdx.json", "2".repeat(64)],
]

function evidence(overrides = {}) {
  const manifestText = payloads.map(([name, digest]) => `${digest}  ${name}`).join("\n") + "\n"
  const manifestDigest = createHash("sha256").update(manifestText).digest("hex")
  const receiptName = `darkphish-v${version}.release.json`
  const receipt = {
    schema: "darkphish-release-publication-receipt/v1",
    tag: `v${version}`,
    source_sha: source,
    checksums_sha256: manifestDigest,
    ...overrides.receipt,
  }
  const assets = [
    ...payloads.map(([name, digest]) => ({ name, digest: `sha256:${digest}` })),
    { name: "SHA256SUMS", digest: `sha256:${manifestDigest}` },
    { name: receiptName, digest: `sha256:${"3".repeat(64)}` },
  ]
  const local = new Map([
    ...payloads.map(([name, digest]) => [name, { digest }]),
    ["SHA256SUMS", { digest: manifestDigest }],
    [receiptName, { digest: "3".repeat(64) }],
  ])
  return { version, source, assets, local, manifestText: overrides.manifestText ?? manifestText, receiptText: `${JSON.stringify(receipt)}\n` }
}

test("receipt-backed staged artifacts bind immutable source and every payload digest", () => {
  assert.equal(verifyReceiptBackedArtifacts(evidence()), true)
})

test("receipt-backed staged artifacts fail closed on source, receipt or manifest changes", () => {
  assert.throws(() => verifyReceiptBackedArtifacts({ ...evidence(), source: "a".repeat(40) }), /immutable source/)
  assert.throws(() => verifyReceiptBackedArtifacts(evidence({ receipt: { tag: "v0.7.2" } })), /immutable source/)
  assert.throws(() => verifyReceiptBackedArtifacts(evidence({ manifestText: `${"4".repeat(64)}  darkphish-v0.7.1-linux-amd64.tar.gz\n${"2".repeat(64)}  darkphish-v0.7.1.spdx.json\n` })), /cryptographically bound/)
})

function step(name) { return { name, status: "completed", conclusion: "success" } }
function jobs() {
  return [
    { name: "metadata", conclusion: "success", steps: [step("Verify historical release provenance and recovery source")] },
    { name: "verify", conclusion: "success", steps: [] },
    { name: "audit-smoke", conclusion: "success", steps: [] },
    ...Array.from({ length: 5 }, (_, index) => ({ name: `binaries (${index})`, conclusion: "success", steps: [] })),
    { name: "publish", conclusion: "success", steps: [
      step("Generate recovery checksums"),
      step("Generate recovery publication receipt"),
      step("Attest rebuilt recovery artifacts"),
      step("Publish rebuilt verified recovery assets"),
      step("Verify published recovery including receipt attestation"),
    ] },
  ]
}

test("only a complete successful recovery pipeline proves staged artifacts", () => {
  assert.equal(recoveryJobsProveStagedArtifacts(jobs()), true)
  const missingReceipt = jobs()
  missingReceipt.at(-1).steps = missingReceipt.at(-1).steps.filter((item) => item.name !== "Generate recovery publication receipt")
  assert.equal(recoveryJobsProveStagedArtifacts(missingReceipt), false)
  const failedBinary = jobs()
  failedBinary.find((job) => job.name === "binaries (2)").conclusion = "failure"
  assert.equal(recoveryJobsProveStagedArtifacts(failedBinary), false)
})
