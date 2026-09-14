import { appendFileSync, existsSync, readFileSync } from "node:fs"
import { pathToFileURL } from "node:url"
import { versionTag } from "./release-lib.mjs"

const path = new URL("../.github/release-normalization-hold.json", import.meta.url)
const output = (name, value) => { if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, `${name}=${value}\n`) }

export function normalizationHold() {
  if (!existsSync(path)) return null
  let value
  try { value = JSON.parse(readFileSync(path, "utf8")) } catch { throw new Error("release normalization hold is malformed JSON") }
  const keys = Object.keys(value || {}).sort().join("\n")
  if (keys !== ["schema", "source_sha", "version"].sort().join("\n")) throw new Error("release normalization hold has unexpected fields")
  if (value.schema !== "darkphish-release-normalization-hold/v1") throw new Error("release normalization hold schema is unsupported")
  versionTag(value.version)
  if (!/^[a-f0-9]{40}$/.test(value.source_sha || "")) throw new Error("release normalization hold source SHA is invalid")
  const current = readFileSync(new URL("../VERSION", import.meta.url), "utf8").trim()
  if (value.version !== current) throw new Error("release normalization hold does not match current VERSION")
  return Object.freeze({ version: value.version, source: value.source_sha, tag: versionTag(value.version) })
}

function cli() {
  const command = process.argv[2] || "check"
  if (command !== "check") throw new Error(`unknown release normalization hold command ${command}`)
  const hold = normalizationHold()
  output("active", hold ? "true" : "false")
  if (hold) {
    output("version", hold.version); output("source", hold.source); output("tag", hold.tag)
    console.log(`Release normalization hold is active for ${hold.tag} at ${hold.source}.`)
  } else console.log("No release normalization hold is active.")
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) cli()
