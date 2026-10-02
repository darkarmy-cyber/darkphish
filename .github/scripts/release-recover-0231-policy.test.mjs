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
test("tagless v0.23.1 recovery selects only PR181",()=>{ assert.equal(auditedTagless0231Source(prFixture()),source); for(const mutate of [
 v=>v.pr.number++,v=>v.pr.merge_commit_sha="a".repeat(40),v=>v.pr.merged_by.id++,v=>v.pr.head.sha="b".repeat(40),v=>v.pr.base.sha="c".repeat(40)
]){const v=prFixture();mutate(v);assert.equal(auditedTagless0231Source(v),null)}})
test("v0.23.1 metadata policy rejects malformed candidates",()=>{ assert.equal(audited0231MetadataFailure({repository,source}),false) })
