import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import vm from 'node:vm'
const png='data:image/png;base64,iVBORw0KGgo='
function fixture(path, count=1) {
  const nodes=Array.from({length:count},(_,i)=>({attrs:{'data-training-image-url':'https://images.example.test/'+i+'.png'},getAttribute(k){return this.attrs[k]},setAttribute(k,v){this.attrs[k]=v}}))
  const elements=new Map(), requests=[]
  const $=key=>{if(!elements.has(key))elements.set(key,{text(v){this.value=v;return this},prop(k,v){this[k]=v;return this}});return elements.get(key)}
  const editor={mode:'wysiwyg',document:{$:{querySelectorAll:()=>nodes}},fire(){},setMode(mode,callback){this.mode=mode;callback()}}
  const ctx=vm.createContext({$,URL,trainingStatic:true,CKEDITOR:{instances:{html_editor:editor}},api:{preview_email_images(body){
    const callbacks={}, request={body,done(fn){callbacks.done=fn;return this},fail(fn){callbacks.fail=fn;return this},always(fn){callbacks.always=fn;return this},
      resolve(data){callbacks.done(data);callbacks.always()},reject(){callbacks.fail();callbacks.always()}}
    requests.push(request);return request
  }}})
  vm.runInContext(readFileSync(new URL('../'+path,import.meta.url),'utf8'),ctx)
  return{ctx,nodes,requests,$,helper:ctx.trainingImagePreview,editor}
}
for(const file of ['static/js/src/app/training_images.js','static/js/dist/app/training_images.min.js']) {
  test(file+': explicit consent, raster-only data, serial requests and static-only guard',()=>{
    const p=fixture(file)
    p.helper.reset();assert.equal(p.requests.length,0)
    p.ctx.trainingStatic=false;p.helper.load();assert.equal(p.requests.length,0)
    p.ctx.trainingStatic=true;p.helper.load();p.helper.load();assert.equal(p.requests.length,1)
    p.requests[0].resolve([{url:'https://images.example.test/0.png',data:png}])
    assert.equal(p.nodes[0].attrs.src,png)
    assert.equal(p.nodes[0].attrs['data-cke-saved-src'],png)
    p.helper.load();assert.equal(p.requests.length,1)
  })
  test(file+': failed first batch cannot starve later images; late replies cannot cross pages',()=>{
    const p=fixture(file,13);p.helper.load()
    assert.equal(p.requests[0].body.urls.length,12)
    p.requests[0].resolve([]);p.helper.load()
    assert.deepEqual([...p.requests[1].body.urls],['https://images.example.test/12.png'])
    p.helper.reset()
    p.requests[1].resolve([{url:'https://images.example.test/12.png',data:png}])
    assert.equal(p.nodes[12].attrs.src,undefined)
    p.helper.load();p.requests[2].reject()
    assert.equal(p.$('#loadTrainingImages').disabled,false)
  })
  test(file+': malformed and untrusted responses cannot inject active content',()=>{
    const p=fixture(file)
    p.helper.load();p.requests[0].resolve([{url:'https://images.example.test/0.png',data:'data:image/svg+xml,<script/>'}])
    assert.equal(p.nodes[0].attrs.src,undefined)
    p.helper.load();p.requests[1].resolve(null)
    assert.match(p.$('#trainingImageStatus').value,/Unexpected/)
    assert.equal(p.$('#loadTrainingImages').disabled,false)
  })
}
