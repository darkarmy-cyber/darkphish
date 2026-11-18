import assert from "node:assert/strict"
import test from "node:test"
import { audited0271MetadataOnly, audited0271Draft, audited0271QuarantinedAsset } from "./release-recover-0271-policy.mjs"
const source = "28a3de27bef29224a23d818dfe61db9f47d5711e"
function fixture() {
  const base = { run_id: 43911299954, run_attempt: 1, head_sha: source, status: "completed" }
  const job = (id, name, conclusion, start, end) => ({ ...base, id, name, conclusion, started_at:start, completed_at:end, steps:[] })
  const m = job(133477034203,"metadata","success","2026-11-16T17:23:54Z","2026-11-16T17:24:14Z")
  m.steps = [
    [1,"Set up job"],[2,"Run actions/checkout@v7"],[3,"Run actions/setup-node@v7"],
    [4,"Run node scripts/changelog.mjs validate"],[5,"Enforce release normalization hold"],
    [6,"Run node scripts/release-publish.mjs metadata"],[11,"Post Run actions/setup-node@v7"],
    [12,"Post Run actions/checkout@v7"],[13,"Complete job"],
  ].map(([number,name])=>({number,name,status:"completed",conclusion:"success"}))
  return {repository:"darkarmy-cyber/darkphish",source,run:{
    id:43911299954, run_number:792, run_attempt:1, workflow_id:351159930,
    name:"Native release",path:".github/workflows/release.yml",event:"workflow_run",
    head_branch:"main",head_sha:source,status:"completed",conclusion:"success",
    created_at:"2026-11-16T16:47:05Z",updated_at:"2026-11-16T17:24:16Z",
    actor:{login:"oliverkko",id:309485696,type:"User"},
  },jobs:[
    m,
    job(133477284605,"verify","skipped","2026-11-16T17:24:15Z","2026-11-16T17:24:15Z"),
    job(133477286539,"binaries","skipped","2026-11-16T17:24:15Z","2026-11-16T17:24:15Z"),
    job(133477287908,"audit-smoke","skipped","2026-11-16T17:24:15Z","2026-11-16T17:24:15Z"),
    job(133477288121,"publish","skipped","2026-11-16T17:24:15Z","2026-11-16T17:24:15Z"),
  ]}
}
test("only the audited 0.27.1 metadata-only run may seed a new recovery rebuild",()=>{
 assert.equal(audited0271MetadataOnly(fixture()),true)
 for (const mutate of [
  x=>{x.source="a".repeat(40)},x=>{x.run.id++},x=>{x.run.run_number++},
  x=>{x.run.conclusion="failure"},x=>{x.run.event="schedule"},
  x=>{x.run.actor.id++},x=>{x.run.updated_at="2026-11-17T17:24:16Z"},
  x=>{x.jobs[0].conclusion="failure"},x=>{x.jobs[1].conclusion="success"},
  x=>{x.jobs[0].steps[5].conclusion="failure"},x=>{x.jobs[0].steps.pop()},
  x=>{x.jobs[0].started_at="2026-11-16T17:23:55Z"},
  x=>{x.jobs.pop()},
 ]) { const v=fixture();mutate(v);assert.equal(audited0271MetadataOnly(v),false) }
})
test("quarantined 0.27.1 assets require exact signed release identity and fingerprints",()=>{
 const rel={id:447354479,tag_name:"v0.27.1",target_commitish:source,draft:true,prerelease:false,author:{login:"github-actions[bot]",type:"Bot",id:41898282}}
 const asset={id:755730348,name:"SHA256SUMS",size:610,digest:"sha256:92848be31ea07c45d71d40affe628809d14bff33b5783908a178f378fc07a138",state:"uploaded",uploader:{login:"github-actions[bot]",type:"Bot",id:41898282}}
 assert.equal(audited0271Draft(rel,"0.27.1",source),true)
 assert.equal(audited0271QuarantinedAsset(rel,"0.27.1",source,asset),true)
 for(const change of [
  a=>{a.id++},a=>{a.size++},a=>{a.digest="sha256:"+"a".repeat(64)},
  a=>{a.name="something-else"},a=>{a.state="starter"},
  a=>{a.uploader.id++},
 ]){const a=structuredClone(asset);change(a);assert.equal(audited0271QuarantinedAsset(rel,"0.27.1",source,a),false)}
 assert.equal(audited0271QuarantinedAsset({...rel,draft:false},"0.27.1",source,asset),false)
 assert.equal(audited0271QuarantinedAsset(rel,"0.27.2",source,asset),false)
})
