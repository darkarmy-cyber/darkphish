import { pathToFileURL } from "node:url"
import { api, git, mergeReviewedPullRequest, pages, protectedMain, repository } from "./release-lib.mjs"

export function eligibleEngineeringPR(repo, pr) {
  return pr?.state === "open" && pr.base?.ref === "main" && pr.head?.repo?.full_name === repo &&
    ["OWNER", "MEMBER", "COLLABORATOR"].includes(pr.author_association)
}
async function reconcile() {
  const repo = repository()
  // The workflow checks out protected main, never PR head or a PR artifact.
  await protectedMain(repo, git("rev-parse", "HEAD"))
  const number = process.env.TARGET_PR_NUMBER || ""
  if (number && !/^[1-9]\d*$/.test(number)) throw new Error("Invalid pull request number")
  if (number && !Number.isSafeInteger(Number(number))) throw new Error("Invalid pull request number")
  const candidates = number ? [await api(`repos/${repo}/pulls/${number}`)] :
    (await pages(`repos/${repo}/pulls?state=open&base=main`)).filter(pr => pr.labels?.some(label => label.name === "codex-automerge"))
  for (const pr of candidates) {
    // Bot-generated release PRs belong to release-prepare's exact-generator
    // recovery path. No dependency PR is implicitly opted in here.
    if (eligibleEngineeringPR(repo, pr)) await mergeReviewedPullRequest(repo, pr)
  }
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  reconcile().catch(error => { console.error(error.message); process.exitCode = 1 })
}
