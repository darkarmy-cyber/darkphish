import assert from "node:assert/strict"
import test from "node:test"
import { verifyV0272Repair } from "./release-repair-0272.mjs"

const repo = "darkarmy-cyber/darkphish"
const head = "a".repeat(40)
const paths = [
  ["docs/RECOVERY_V0_27_2.md", "added"],
  ["changes/recover-v0-27-2-attestation.md", "added"],
  ["scripts/release-repair-policy.mjs", "modified"],
  ["scripts/release-repair-0272.mjs", "added"],
  ["scripts/release-repair-0272.test.mjs", "added"],
]
const fragment = "---\ncategory: Security\nversion: 0.27.2\n---\n- Restore the v0.27.2 release publication path by requiring a new generated release pull request with complete pre-merge security and maintainer review attestations; the original unreviewed release merge remains unpublished.\n"
function fixture() {
  const pr = {
    number: 247, state: "open", draft: false,
    title: "fix(release): recover v0.27.2 through fresh attested release PR",
    base: {ref:"main",sha:"3f103ded1bcae7e8eccaaf46c81b910c591586cd"},
    head: {ref:"fix/v0-27-2-release-provenance-recovery",sha:head,repo:{full_name:repo}},
    author_association: "OWNER",
  }
  const files = paths.map(([filename,status]) => ({filename,status}))
  const old = {
    state:"closed", merged_at:"2026-11-19T13:55:57Z",
    head:{sha:"cc600c1cfb741a30cf3b4d85917c1d57545ae49c",ref:"release/v0.27.2"},
    merge_commit_sha:"3f103ded1bcae7e8eccaaf46c81b910c591586cd",
    base:{ref:"main"},
  }
  const read = {VERSION:"0.27.2\n","changes/recover-v0-27-2-attestation.md":fragment}
  const request = async (path, options) => {
    if (path.endsWith("pulls/247/files?per_page=100&page=1")) return files
    if (path.endsWith("pulls/244")) return old
    if (path.includes("/git/ref/tags/") || path.includes("/releases/tags/")) return null
    const file = /^repos\/[^/]+\/[^/]+\/contents\/(.+)\?ref=/.exec(path)
    if (file) return {type:"file",encoding:"base64",content:Buffer.from(read[file[1]]??"").toString("base64")}
    throw new Error("unexpected API request "+path)
  }
  return {pr,files,old,read,request,verify:()=>verifyV0272Repair(repo,pr,request)}
}
test("exact absent-tag one-shot v0.27.2 repair is admitted for review only", async()=>{
  await fixture().verify()
})
test("refuse missing or changed PR identity and branch",async()=>{
  for(const mutate of [
    f=>{f.pr.number=248}, f=>{f.pr.draft=true},f=>{f.pr.base.sha="b".repeat(40)},
    f=>{f.pr.head.sha="bad"},f=>{f.pr.author_association="NONE"},
  ]) {const f=fixture();mutate(f);await assert.rejects(f.verify())}
})
test("refuse unexpected paths, versions and fragment content",async()=>{
  for(const mutate of [
    f=>{f.files.push({filename:".github/workflows/release.yml",status:"modified"})},
    f=>{f.files[0].status="removed"},
    f=>{f.read.VERSION="0.27.3\n"},
    f=>{f.read["changes/recover-v0-27-2-attestation.md"]="modified"},
  ]) {const f=fixture();mutate(f);await assert.rejects(f.verify())}
})
test("refuse changed historical evidence and existing publication",async()=>{
  const f=fixture();f.old.merge_commit_sha="b".repeat(40);await assert.rejects(f.verify())
  const g=fixture(),original=g.request
  g.request=async (path,options)=>path.includes("/releases/tags/v0.27.2")?{id:2}:original(path,options)
  await assert.rejects(g.verify())
  await assert.rejects(verifyV0272Repair("other/repo",fixture().pr,fixture().request))
})
