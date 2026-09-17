import { createHash } from "node:crypto"
import { pages } from "./release-lib.mjs"

// A 30 MB upload has taken 118 seconds and returned HTTP 500 despite GitHub
// retaining the complete asset. Never replay that ambiguous, non-idempotent POST.
export const uploadTimeoutMs = 180000
const actionsBot = (actor) => actor?.login === "github-actions[bot]" && actor?.type === "Bot" && actor?.id === 41898282

export async function uploadReleaseAsset(repo, release, name, content, {
  request = globalThis.fetch, list = pages,
  pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
  token = process.env.GH_TOKEN || process.env.GITHUB_TOKEN,
} = {}) {
  if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repo) || repo.split("/").some((part) => part === "." || part === "..")) throw new Error("invalid upload repository")
  if (!Number.isSafeInteger(release?.id) || release.id < 1 || release.draft !== true) throw new Error("upload requires an identified private draft")
  if (!/^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(name) || !Buffer.isBuffer(content) || !token) throw new Error("invalid asset upload input")
  const url = new URL(release.upload_url.split("{")[0])
  if (url.origin !== "https://uploads.github.com" || url.username || url.password || url.search || url.hash || url.pathname !== `/repos/${repo}/releases/${release.id}/assets`) throw new Error("unexpected release upload destination")
  url.searchParams.set("name", name)
  const digest = `sha256:${createHash("sha256").update(content).digest("hex")}`
  let response, failure = "transport error"
  try {
    response = await request(url, {
      method: "POST", redirect: "error", signal: AbortSignal.timeout(uploadTimeoutMs),
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/octet-stream", "Content-Length": String(content.length) }, body: content,
    })
    failure = `HTTP ${response.status}`
  } catch {
    // An aborted request can still have committed remotely. Only exact read-back
    // evidence may accept it; no second POST, DELETE, overwrite or public PATCH.
  } finally {
    // Release fetch connection resources even for HTTP failures. Do not log
    // untrusted response bodies or transport errors that could include secrets.
    if (response?.body) { try { await response.body.cancel() } catch { /* read-back remains mandatory */ } }
  }
  if (response && response.status !== 201 && response.status < 500) throw new Error(`asset upload ${name} rejected with ${failure}; draft retained`)
  for (let attempt = 0; attempt < 3; attempt += 1) {
    const assets = await list(`repos/${repo}/releases/${release.id}/assets`)
    if (!Array.isArray(assets)) throw new Error("invalid uploaded asset listing")
    const matches = assets.filter((asset) => asset?.name === name)
    if (matches.length > 1) throw new Error(`ambiguous duplicate uploaded asset ${name}; draft retained`)
    if (matches.length === 1) {
      const asset = matches[0]
      if (!Number.isSafeInteger(asset.id) || asset.id < 1 || !actionsBot(asset.uploader) || asset.state !== "uploaded" || asset.digest !== digest || asset.size !== content.length) throw new Error(`uploaded asset ${name} is incomplete or differs; draft retained`)
      return asset
    }
    if (attempt < 2) await pause(2000)
  }
  throw new Error(`asset upload ${name} not confirmed after ${failure}; draft retained without replay`)
}
