import assert from "node:assert/strict"
import test from "node:test"
import { audited0240MetadataFailure, auditedTagless0240Source } from "./release-recover-0240-policy.mjs"

const repository="darkarmy-cyber/darkphish", source="3f20a49ee02bddff90f03db67edeae0e236cf55f"
function prFixture(){return {repository,version:"0.24.0",tag:"v0.24.0",pr:{
 number:186,state:"closed",draft:false,title:"release: Darkphish 0.24.0",merged_at:"2026-10-02T16:39:39Z",
 merge_commit_sha:source,merged_by:{login:"oliverkko",id:309485696},
 base:{ref:"main",sha:"01f38cccbb8e466239be360cd5682a78a1620560"},
 head:{ref:"release/v0.24.0",sha:"7b808b9ab02b46c6f0ea80b6c382bd1a7acdabad",repo:{full_name:repository}}
}}}
function runFixture(){
 const step=(number,name,conclusion)=>({number,name,status:"completed",conclusion})
 const job=(id,name,conclusion,started_at,completed_at)=>({id,run_id:37035934324,run_attempt:1,head_sha:source,name,status:"completed",conclusion,started_at,completed_at})
 const metadata=job(110935919656,"metadata","failure","2026-10-02T16:49:13Z","2026-10-02T16:49:28Z")
 metadata.steps=[step(1,"Set up job","success"),step(2,"Run actions/checkout@v7","success"),step(3,"Run actions/setup-node@v7","success"),step(4,"Run node scripts/changelog.mjs validate","success"),step(5,"Enforce release normalization hold","success"),step(6,"Run node scripts/release-publish.mjs metadata","failure"),step(11,"Post Run actions/setup-node@v7","skipped"),step(12,"Post Run actions/checkout@v7","success"),step(13,"Complete job","success")]
 return {repository,source,run:{id:37035934324,run_number:484,run_attempt:1,workflow_id:351159930,name:"Native release",path:".github/workflows/release.yml",event:"workflow_run",head_branch:"main",head_sha:source,status:"completed",conclusion:"failure",created_at:"2026-10-02T16:44:09Z",updated_at:"2026-10-02T16:49:29Z",actor:{login:"oliverkko",id:309485696,type:"User"}},jobs:[
  metadata,
  job(110936033864,"verify","skipped","2026-10-02T16:49:29Z","2026-10-02T16:49:28Z"),
  job(110936034987,"binaries","skipped","2026-10-02T16:49:29Z","2026-10-02T16:49:28Z"),
  job(110936035915,"audit-smoke","skipped","2026-10-02T16:49:29Z","2026-10-02T16:49:29Z"),
  job(110936036722,"publish","skipped","2026-10-02T16:49:29Z","2026-10-02T16:49:29Z"),
 ]}
}
test("tagless v0.24.0 recovery selects only PR186",()=>{assert.equal(auditedTagless0240Source(prFixture()),source);for(const mutate of [v=>v.pr.number++,v=>v.pr.merge_commit_sha="a".repeat(40),v=>v.pr.merged_by.id++,v=>v.pr.head.sha="b".repeat(40),v=>v.pr.base.sha="c".repeat(40)]){const v=prFixture();mutate(v);assert.equal(auditedTagless0240Source(v),null)}})
test("historical provenance accepts only Native release run 484 exact metadata failure",()=>{assert.equal(audited0240MetadataFailure(runFixture()),true);for(const mutate of [v=>v.source="a".repeat(40),v=>v.run.id++,v=>v.run.run_attempt=2,v=>v.run.event="schedule",v=>v.run.updated_at="2026-10-02T16:49:30Z",v=>v.run.actor.id++,v=>v.jobs[0].run_attempt=2,v=>v.jobs[0].head_sha="b".repeat(40),v=>v.jobs[0].steps[5].conclusion="success",v=>v.jobs[1].conclusion="success",v=>v.jobs.pop()]){const v=runFixture();mutate(v);assert.equal(audited0240MetadataFailure(v),false)}})
