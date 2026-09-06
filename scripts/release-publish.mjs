import { appendFileSync, readFileSync, readdirSync, statSync } from "node:fs"
import { createHash } from "node:crypto"
import { api, assetDisposition, assertReleaseState, generatedPath, git, greenCommit, pages, protectedMain, repository, verifyChecksums, versionTag } from "./release-lib.mjs"
import { verifyCodeQLBaseline } from "./codeql-baseline.mjs"

async function source() {
  const repo = repository()
  const sha = git("rev-parse", "HEAD")
  const version = readFileSync("VERSION", "utf8").trim()
  const tag = versionTag(version)
  await protectedMain(repo, sha)
  if (!await greenCommit(repo, sha)) return null
  const prs = await pages(`repos/${repo}/commits/${sha}/pulls`)
  const pr = prs.find((item) => item.merged_at && item.merge_commit_sha === sha && item.base.ref === "main" && item.head.ref === `release/${tag}` && item.title === `release: Darkphish ${version}`)
  if (!pr) return null
  await verifyCodeQLBaseline(repo, sha)
  const files = await pages(`repos/${repo}/pulls/${pr.number}/files`)
  if (!files.length || files.some((file) => !generatedPath(file.filename))) throw new Error("release PR includes application changes")
  if (readdirSync("changes").some((name) => name.endsWith(".md") && name !== "README.md")) throw new Error("release source contains unconsumed fragments")
  const changelog = readFileSync("CHANGELOG.md", "utf8")
  const start = changelog.indexOf(`## ${version} - `)
  if (start < 0) throw new Error("release changelog is missing")
  const end = changelog.indexOf("\n## ", start + 1)
  let ref = await api(`repos/${repo}/git/ref/tags/${tag}`, { missing: true })
  if (ref?.object.type === "tag") ref = await api(`repos/${repo}/git/tags/${ref.object.sha}`)
  const release = await api(`repos/${repo}/releases/tags/${tag}`, { missing: true })
  const state = assertReleaseState(ref?.object.sha, release, sha)
  return { repo, sha, version, tag, release, state, notes: changelog.slice(start, end < 0 ? undefined : end).trim(), builtAt: git("show", "-s", "--format=%cI", sha) }
}

async function run() {
  const command = process.argv[2]
  const current = await source()
  if (command === "metadata") {
    const ready = current && current.state !== "published"
    const output = ready ? `ready=true\nversion=${current.version}\ntag=${current.tag}\nsha=${current.sha}\nbuilt_at=${new Date(current.builtAt).toISOString()}\n` : "ready=false\n"
    if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, output)
    console.log(ready ? `Verified protected release source ${current.sha}` : "No unpublished, fully checked release merge is ready.")
    return
  }
  if (command !== "publish" || !current) throw new Error("publication requires the fully checked protected release merge")
  if (current.state === "published") {
    console.log("Release already published; leaving all assets immutable.")
    return
  }
  const { repo, sha, version, tag } = current
  const expected = ["linux-amd64.tar.gz", "linux-arm64.tar.gz", "windows-amd64.zip", "darwin-amd64.tar.gz", "darwin-arm64.tar.gz"].map((target) => `darkphish-${tag}-${target}`)
  expected.push(`darkphish-${tag}.spdx.json`, "SHA256SUMS")
  const names = readdirSync("dist").filter((name) => statSync(`dist/${name}`).isFile()).sort()
  if (names.join("\n") !== expected.sort().join("\n")) throw new Error("native release artifact set is incomplete or unexpected")
  const bytes = new Map(names.map((name) => [name, readFileSync(`dist/${name}`)]))
  const hashes = new Map([...bytes].map(([name, value]) => [name, createHash("sha256").update(value).digest("hex")]))
  verifyChecksums(readFileSync("dist/SHA256SUMS", "utf8"), hashes)
  let release = current.release
  if (!release) {
    release = await api(`repos/${repo}/releases`, { method: "POST", body: {
      tag_name: tag, target_commitish: sha, name: `Darkphish ${version.split(".").slice(0, 2).join(".")}`,
      draft: true, body: `${current.notes}\n\nSource commit: ${sha}\n\n<!-- darkphish-release-source:${sha} -->\n\nNative binaries, SHA-256 checksums and SPDX SBOM are attached. Artifact attestations require GitHub Enterprise Cloud for this private repository; see docs/REPOSITORY_ADMIN.md.`,
    } })
  }
  const assets = await pages(`repos/${repo}/releases/${release.id}/assets`)
  if (assets.some((asset) => !names.includes(asset.name)) || new Set(assets.map((asset) => asset.name)).size !== assets.length) throw new Error("draft contains unexpected release assets; refusing to publish")
  for (const name of names) {
    const content = bytes.get(name)
    if (assetDisposition(assets.find((asset) => asset.name === name), hashes.get(name), content.length) === "reuse") continue
    const url = new URL(release.upload_url.split("{")[0])
    if (url.origin !== "https://uploads.github.com") throw new Error("unexpected release upload host")
    url.searchParams.set("name", name)
    const response = await fetch(url, { method: "POST", headers: { Authorization: `Bearer ${process.env.GH_TOKEN}`, "Content-Type": "application/octet-stream" }, body: content })
    if (!response.ok) throw new Error(`asset upload failed with HTTP ${response.status}; draft retained for safe retry`)
  }
  const uploaded = await pages(`repos/${repo}/releases/${release.id}/assets`)
  if (uploaded.length !== names.length) throw new Error("unexpected release asset set after upload")
  for (const name of names) {
    if (!uploaded.some((asset) => asset.name === name)) throw new Error("release asset missing after upload")
    assetDisposition(uploaded.find((asset) => asset.name === name), hashes.get(name), bytes.get(name).length)
  }
  await protectedMain(repo, sha)
  await verifyCodeQLBaseline(repo, sha)
  await api(`repos/${repo}/releases/${release.id}`, { method: "PATCH", body: { draft: false, make_latest: "true" } })
  console.log(`Published https://github.com/${repo}/releases/tag/${tag}`)
}
run().catch((error) => { console.error(error.message); process.exitCode = 1 })
