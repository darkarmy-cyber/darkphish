import assert from "node:assert/strict"
import test from "node:test"
import { audited0231MetadataFailure, auditedTagless0231Source } from "./release-recover-0231-policy.mjs"

const repository="darkarmy-cyber/darkphish", source="86cb7173c553d68bc3afac204948aa923a32bc83"

function prFixture(){ return {repository,version:"0.23.1",tag:"v0.23.1",pr:{
  number:181,state:"closed",draft:false,title:"release: Darkphish 0.23.1",merged_at:"2026-10-02T12:24:22Z",
  merge_commit_sha:source,merged_by:{login:"oliverkko",id:309485696},
  base:{ref:"main",sha:"443d247b1636ea8e46c7339378174d3c88a0abf5"},
  head:{ref:"release/v0.23.1",sha:"5f8b6de5a6ed4076b987747c32d615b5b9bd180a",repo:{full_name:repository}}
}}}

function runFixture(){
  const step=(number,name,conclusion)=>({number,name,status:"completed",conclusion})
  const job=(id,name,conclusion,started_at="2026-10-02T12:54:51Z")=>({
    id,run_id:37009036246,run_attempt:2,head_sha:source,name,status:"completed",conclusion,
    started_at,completed_at:"2026-10-02T12:54:51Z",
  })
  const metadata=job(110845725168,"metadata","failure","2026-10-02T12:54:34Z")
  metadata.steps=[
    step(1,"Set up job","success"),step(2,"Run actions/checkout@v7","success"),
    step(3,"Run actions/setup-node@v7","success"),step(4,"Run node scripts/changelog.mjs validate","success"),
    step(5,"Enforce release normalization hold","success"),step(6,"Run node scripts/release-publish.mjs metadata","failure"),
    step(11,"Post Run actions/setup-node@v7","skipped"),step(12,"Post Run actions/checkout@v7","success"),
    step(13,"Complete job","success"),
  ]
  return {repository,source,run:{
    id:37009036246,run_number:474,run_attempt:2,workflow_id:351159930,name:"Native release",
    path:".github/workflows/release.yml",event:"schedule",head_branch:"main",head_sha:source,
    status:"completed",conclusion:"failure",created_at:"2026-10-02T12:49:30Z",updated_at:"2026-10-02T12:54:52Z",
    actor:{login:"github-actions[bot]",id:41898282,type:"Bot"},
    triggering_actor:{login:"oliverkko",id:309485696,type:"User"},
  },jobs:[
    metadata,
    job(110845838722,"verify","skipped"),
    job(110845839460,"binaries","skipped"),
    job(110845839732,"audit-smoke","skipped"),
    job(110845839869,"publish","skipped"),
  ]}
}

test("tagless v0.23.1 recovery selects only PR181",()=>{
  assert.equal(auditedTagless0231Source(prFixture()),source)
  for(const mutate of [
    v=>v.pr.number++,v=>v.pr.merge_commit_sha="a".repeat(40),v=>v.pr.merged_by.id++,
    v=>v.pr.head.sha="b".repeat(40),v=>v.pr.base.sha="c".repeat(40)
  ]){const v=prFixture();mutate(v);assert.equal(auditedTagless0231Source(v),null)}
})

test("historical provenance accepts only Native release run 474 attempt 2 exact metadata failure",()=>{
  assert.equal(audited0231MetadataFailure(runFixture()),true)
  for(const mutate of [
    v=>v.source="a".repeat(40),v=>v.run.id++,v=>v.run.run_attempt=1,v=>v.run.event="workflow_run",
    v=>v.run.updated_at="2026-10-02T12:54:53Z",v=>v.run.actor.id++,v=>v.run.triggering_actor.id++,
    v=>v.jobs[0].run_attempt=1,v=>v.jobs[0].head_sha="b".repeat(40),
    v=>v.jobs[0].steps[5].conclusion="success",v=>v.jobs[1].conclusion="success",v=>v.jobs.pop()
  ]){const v=runFixture();mutate(v);assert.equal(audited0231MetadataFailure(v),false)}
})
