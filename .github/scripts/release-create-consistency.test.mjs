import assert from "node:assert/strict"
import test from "node:test"

const endpoint = "https://api.github.com/repos/darkarmy-cyber/darkphish/releases"
const created = { id: 388023823, tag_name: "v0.7.1", draft: true, prerelease: false }

async function withGuard(responses, run) {
  const previous = globalThis.fetch
  const calls = []
  globalThis.fetch = async (input, init = {}) => {
    calls.push({ input: String(input), init })
    const next = responses.shift()
    if (!next) throw new Error("unexpected fetch call")
    return new Response(JSON.stringify(next.body), {
      status: next.status || 200,
      headers: { "content-type": "application/json" },
    })
  }
  try {
    await import(`./release-create-consistency.mjs?test=${Date.now()}-${Math.random()}`)
    await run(calls)
  } finally {
    globalThis.fetch = previous
  }
}

test("release POST waits until the created draft is visible in the collection", async () => {
  await withGuard([
    { body: created },
    { body: [] },
    { body: [created] },
  ], async (calls) => {
    const response = await globalThis.fetch(endpoint, {
      method: "POST",
      headers: { authorization: "Bearer test", "x-github-api-version": "2022-11-28" },
      body: "{}",
    })
    assert.equal(response.status, 200)
    assert.equal(calls.length, 3)
    assert.equal(calls[0].input, endpoint)
    assert.equal(calls[1].input, `${endpoint}?per_page=100`)
    assert.equal(calls[2].input, `${endpoint}?per_page=100`)
    assert.equal(calls[1].init.method, "GET")
  })
})

test("collection visibility fails closed if the created release is no longer an exact private draft", async () => {
  await withGuard([
    { body: created },
    { body: [{ ...created, draft: false }] },
  ], async () => {
    await assert.rejects(
      globalThis.fetch(endpoint, { method: "POST", headers: { authorization: "Bearer test" }, body: "{}" }),
      /created recovery release changed before collection visibility/,
    )
  })
})

test("only the exact GitHub releases collection POST is intercepted", async () => {
  await withGuard([{ body: { ok: true } }], async (calls) => {
    const response = await globalThis.fetch(`${endpoint}/123`, { method: "POST", body: "{}" })
    assert.equal(response.status, 200)
    assert.equal(calls.length, 1)
  })
})
