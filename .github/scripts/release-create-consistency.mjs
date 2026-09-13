const originalFetch = globalThis.fetch
if (typeof originalFetch !== "function") throw new Error("global fetch is unavailable for release consistency guard")

const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const releasesEndpoint = /^https:\/\/api\.github\.com\/repos\/[^/]+\/[^/]+\/releases$/

function requestMethod(input, init) {
  if (typeof init?.method === "string") return init.method.toUpperCase()
  if (typeof Request !== "undefined" && input instanceof Request) return input.method.toUpperCase()
  return "GET"
}

function requestURL(input) {
  if (typeof input === "string") return input
  if (input instanceof URL) return input.href
  if (typeof Request !== "undefined" && input instanceof Request) return input.url
  return ""
}

async function waitForCollectionVisibility(url, init, created) {
  if (!Number.isSafeInteger(created?.id) || created.id < 1 || created.draft !== true || typeof created.tag_name !== "string" || !created.tag_name) {
    throw new Error("created recovery release draft response is malformed")
  }

  const headers = new Headers(init?.headers || (typeof Request !== "undefined" && init == null ? undefined : undefined))
  for (let attempt = 1; attempt <= 20; attempt += 1) {
    const response = await originalFetch(`${url}?per_page=100`, {
      method: "GET",
      headers,
      redirect: "follow",
      signal: AbortSignal.timeout(10000),
    })
    if (!response.ok) throw new Error(`release collection consistency probe failed with HTTP ${response.status}`)
    const releases = await response.json()
    if (!Array.isArray(releases)) throw new Error("release collection consistency probe returned malformed JSON")
    const matching = releases.find((release) => release?.id === created.id)
    if (matching) {
      if (matching.tag_name !== created.tag_name || matching.draft !== true || matching.prerelease !== false) {
        throw new Error("created recovery release changed before collection visibility")
      }
      return
    }
    await delay(Math.min(2000, 250 * attempt))
  }
  throw new Error("created recovery release did not become visible in the release collection within the bounded consistency window")
}

globalThis.fetch = async (input, init) => {
  const url = requestURL(input)
  const method = requestMethod(input, init)
  const response = await originalFetch(input, init)
  if (method !== "POST" || !releasesEndpoint.test(url) || !response.ok) return response

  let created
  try { created = await response.clone().json() }
  catch { throw new Error("created recovery release response is not valid JSON") }
  await waitForCollectionVisibility(url, init, created)
  return response
}
