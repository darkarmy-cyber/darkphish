import assert from "node:assert/strict"
import test from "node:test"
import { readFileSync } from "node:fs"
import { reconcilePublishedReleases } from "./release-reconcile.mjs"

test("historical and incomplete staging drafts never reach the metadata mutator", async () => {
  const seen = [], messages = []
  const inventory = [
    { id: 388809146, draft: true, tag_name: "v0.8.0" },
    { id: 388031151, draft: true, tag_name: "v0.7.1", published_at: "2026-09-14T12:00:00Z" },
    { id: 390565224, draft: false, tag_name: "v0.12.0" },
    { id: 390565225, draft: true, tag_name: "v0.13.0" },
    { id: 388815291, draft: false, tag_name: "v0.8.0" },
  ]
  const original = structuredClone(inventory)
  const count = await reconcilePublishedReleases("owner/repo", inventory, "main", {
    reconcileOne: async (repo, release, main) => {
      assert.equal(repo, "owner/repo"); assert.equal(main, "main")
      assert.equal(release.draft, false); seen.push(release.id)
      return release.id === 388815291
    }, log: message => messages.push(message),
  })
  assert.equal(count, 1)
  assert.deepEqual(seen, [390565224, 388815291])
  assert.equal(messages.length, 3)
  assert.deepEqual(inventory, original)
})

test("maintenance propagates verification failures instead of skipping untrusted public releases", async () => {
  await assert.rejects(reconcilePublishedReleases("o/r", [{ id: 1, draft: false }], "main", {
    reconcileOne: async () => { throw new Error("changed source or asset") },
  }), /changed source or asset/)
  await assert.rejects(reconcilePublishedReleases("o/r", null, "main"), /inventory/)
  for (const draft of [undefined, null, "false", 0]) {
    await assert.rejects(reconcilePublishedReleases("o/r", [{ id: 1, draft }], "main"), /draft state/)
  }
})

test("completed historical repairs are explicit main-only manual operations", () => {
  for (const [name, confirmation] of [
    ["release-resume", "resume-original-071"],
    ["release-reconcile", "reconcile-public-metadata"],
    ["release-title-repair", "repair-071-title"],
  ]) {
    const text = readFileSync(new URL(`../.github/workflows/${name}.yml`, import.meta.url), "utf8")
    assert.match(text, /workflow_dispatch:/)
    assert.doesNotMatch(text, /^  (schedule|push|workflow_run|pull_request_target):/m)
    assert.ok(text.includes(`inputs.confirmation == '${confirmation}'`))
    assert.ok(text.includes("github.ref == 'refs/heads/main'"))
    assert.ok(text.indexOf("inputs.confirmation ==") < text.indexOf("uses: actions/checkout"))
    assert.ok(text.includes("ref: ${{ github.sha }}"))
    assert.match(text, /persist-credentials: false/)
  }
})

test("queued automatic mutators still revalidate source and cannot overwrite publication", () => {
  const read = name => readFileSync(new URL(name, import.meta.url), "utf8")
  const publish = read("./release-publish.mjs")
  assert.match(publish, /await protectedMain\(repo, sha\)/)
  assert.match(publish, /await verifyReleaseMaintainerReview/)
  assert.match(publish, /await verifyCodeQLBaseline/)
  assert.match(publish, /finalSource\.state === "published"/)
  assert.match(publish, /assertExactAssetSet/)
  const legacy = read("../.github/workflows/release-title-repair.yml")
  assert.ok(legacy.indexOf("release-reconcile.mjs verify-execution") < legacy.indexOf("gh api --method PATCH"))
})
