import { appendFileSync } from "node:fs"
import { api, generatedPath, repository } from "./release-lib.mjs"
import { verifyCodeQLBaseline } from "./codeql-baseline.mjs"

const repo = repository()
const branch = process.env.TARGET_BRANCH || ""
const sha = process.env.TARGET_SHA || ""
if (!/^release\/v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(branch)) throw new Error("invalid generated release branch")
if (!/^[a-f0-9]{40}$/.test(sha)) throw new Error("invalid generated release SHA")

const owner = repo.split("/")[0]
const ref = await api(`repos/${repo}/git/ref/heads/${branch}`)
if (ref?.object?.sha !== sha) throw new Error("generated release branch moved before trusted CodeQL analysis")

const pulls = await api(`repos/${repo}/pulls?state=open&base=main&head=${encodeURIComponent(`${owner}:${branch}`)}&per_page=10`)
if (!Array.isArray(pulls) || pulls.length !== 1) throw new Error("trusted CodeQL requires exactly one open generated release PR")
const pr = pulls[0]
const version = branch.slice("release/v".length)
if (pr?.draft !== false || pr?.head?.sha !== sha || pr?.head?.ref !== branch || pr?.head?.repo?.full_name !== repo || pr?.base?.ref !== "main" || pr?.title !== `release: Darkphish ${version}` || !/^[a-f0-9]{40}$/.test(pr?.base?.sha || "")) throw new Error("generated release PR identity is not trusted")

const files = await api(`repos/${repo}/pulls/${pr.number}/files?per_page=100`)
if (!Array.isArray(files) || !files.length || files.length >= 100 || files.some((file) => !generatedPath(file?.filename || ""))) throw new Error("generated release PR contains unexpected or ambiguous files")

// Generated release PRs contain no application changes. Bind their exact-head
// analysis to the current protected default-branch baseline so previously
// reviewed/dismissed false positives are not silently converted back into open
// release blockers, while every unresolved/new CodeQL alert still fails closed.
await verifyCodeQLBaseline(repo, pr.base.sha)

const finalRef = await api(`repos/${repo}/git/ref/heads/${branch}`)
const finalPR = await api(`repos/${repo}/pulls/${pr.number}`)
if (finalRef?.object?.sha !== sha || finalPR?.head?.sha !== sha || finalPR?.base?.sha !== pr.base.sha || finalPR?.state !== "open") throw new Error("generated release target changed during trusted CodeQL validation")

if (!process.env.GITHUB_OUTPUT) throw new Error("GITHUB_OUTPUT is required")
appendFileSync(process.env.GITHUB_OUTPUT, `target_sha=${sha}\ntarget_branch=${branch}\nbase_sha=${pr.base.sha}\n`)
console.log(`Validated generated release PR #${pr.number} at ${sha} against protected CodeQL baseline ${pr.base.sha}.`)
