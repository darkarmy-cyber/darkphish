import assert from "node:assert/strict"
import test from "node:test"
import { audited0252MetadataFailure, auditedTagless0252Source } from "./release-recover-0252-policy.mjs"

const repository="darkarmy-cyber/darkphish", source="eb0fe18b9fde19fea159961cb9e3c863cb58b437"
function prFixture(){return {repository,version:"0.25.2",tag:"v0.25.2",pr:{
 number:206,state:"closed",draft:false,title:"release: Darkphish 0.25.2",merged_at:"2026-10-03T20:45:16Z",
 merge_commit_sha:source,merged_by:{login:"oliverkko",id:309485696},
 base:{ref:"main",sha:"a9bae16aa396edd3f19856142abbd763add09674"},
 head:{ref:"release/v0.25.2",sha:"5e10e7f894b62f84fbe0685f8cffd631bda328ed",repo:{full_name:repository}}
}}}
function runFixture(){
 const step=(number,name,conclusion)=>({number,name,status:"completed",conclusion})
 const job=(id,name,conclusion)=>({id,run_id:37152934161,run_attempt:1,head_sha:source,name,status:"completed",conclusion})
 const metadata=job(111290349984,"metadata","failure")
 metadata.steps=[step(1,"Set up job","success"),step(2,"Run actions/checkout@v7","success"),step(3,"Run actions/setup-node@v7","success"),step(4,"Run node scripts/changelog.mjs validate","success"),step(5,"Enforce release normalization hold","success"),step(6,"Run node scripts/release-publish.mjs metadata","failure"),step(11,"Post Run actions/setup-node@v7","skipped"),step(12,"Post Run actions/checkout@v7","success"),step(13,"Complete job","success")]
 return {repository,source,run:{id:37152934161,run_number:515,run_attempt:1,workflow_id:351159930,name:"Native release",path:".github/workflows/release.yml",event:"workflow_run",head_branch:"main",head_sha:source,status:"completed",conclusion:"failure",created_at:"2026-10-03T20:49:48Z",updated_at:"2026-10-03T20:50:03Z",actor:{login:"oliverkko",id:309485696,type:"User"}},jobs:[
  metadata,job(111290392359,"verify","skipped"),job(111290392376,"binaries","skipped"),job(111290392792,"audit-smoke","skipped"),job(111290392908,"publish","skipped")
 ]}
}
test("tagless v0.25.2 recovery selects only PR206",()=>{assert.equal(auditedTagless0252Source(prFixture()),source);for(const mutate of [v=>v.pr.number++,v=>v.pr.merge_commit_sha="a".repeat(40),v=>v.pr.merged_by.id++,v=>v.pr.head.sha="b".repeat(40),v=>v.pr.base.sha="c".repeat(40)]){const v=prFixture();mutate(v);assert.equal(auditedTagless0252Source(v),null)}})
test("historical provenance accepts only Native release run 515 exact metadata failure",()=>{assert.equal(audited0252MetadataFailure(runFixture()),true);for(const mutate of [v=>v.source="a".repeat(40),v=>v.run.id++,v=>v.run.run_attempt=2,v=>v.run.event="schedule",v=>v.run.updated_at="2026-10-03T20:50:04Z",v=>v.run.actor.id++,v=>v.jobs[0].run_attempt=2,v=>v.jobs[0].head_sha="b".repeat(40),v=>v.jobs[0].steps[5].conclusion="success",v=>v.jobs[1].conclusion="success",v=>v.jobs.pop()]){const v=runFixture();mutate(v);assert.equal(audited0252MetadataFailure(v),false)}})
