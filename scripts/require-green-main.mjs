import { api, greenCommit, protectedMain, repository } from "./release-lib.mjs"
const repo = repository()
const branch = await api(`repos/${repo}/branches/main`)
const metadata = await protectedMain(repo, branch.commit.sha)
if (!metadata.allow_auto_merge || !await greenCommit(repo, branch.commit.sha)) throw new Error("dependency auto-merge is paused until protected main is fully green")
