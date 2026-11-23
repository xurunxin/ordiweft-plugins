import fs from 'node:fs/promises';
import test from 'node:test';
import assert from 'node:assert/strict';
const {generation}=JSON.parse(await fs.readFile(new URL('../release.template.json',import.meta.url),'utf8'));
test('new image and video nodes use canonical models and prefer CLI without duplicate alias choices',()=>{
 assert.equal(generation.nodes.length,2);
 for(const node of generation.nodes){
  const model=generation.models.find(m=>m.id===node.model);
  assert.ok(model&&!model.hidden);
  assert.equal(model.fields.channel.default,'auto');
 }
 const visible=generation.models.filter(m=>!m.hidden);
 assert.equal(new Set(visible.map(m=>m.label)).size,visible.length);
 for(const m of visible)assert.equal(m.fields.channel.default,'auto');
 for(const id of ['seedream-5.5-auto','seedance-2.5-auto'])assert.equal(generation.models.find(m=>m.id===id).hidden,true);
});
test('Flash has its own model ID, reference limits and published video options',async()=>{
 const id='doubao-seedance-2-5-flash-261115',m=generation.models.find(x=>x.id===id);
 assert.equal(m.label,'Seedance 2.5 Flash');
 assert.equal(m.maxInputs,20);
 assert.deepEqual(m.fields.resolution.enum,['480p','720p']);
 assert.deepEqual(m.fields.duration.enum,[-1,...Array.from({length:27},(_,i)=>i+4)]);
 const {images,videos,audios}=m.fields.input_mode.inputModes.reference;
 assert.deepEqual([images.max,videos.max,audios.max],[10,5,5]);
 assert.equal(m.fields.draft,undefined);
 assert.equal(m.draftOutput,undefined);
 for(const name of ['generate','quote']){
  const p=JSON.parse(await fs.readFile(new URL('../fixtures/policies/'+name+'.json',import.meta.url),'utf8'));
  assert.equal(p.models[id],id);
 }
});

test('all Seedance ordinary models declare sound while finalization stays tied to its Draft',()=>{
 for(const m of generation.models.filter(m=>m.capability==='video.generate'&&!m.fromDraft))assert.deepEqual(m.fields.generate_audio,{type:'boolean',label:'生成音频'});
 const final=generation.models.find(m=>m.fromDraft);
 assert.ok(final.hidden);
 assert.ok(final.fromDraft.models.includes('doubao-seedance-2-5-260628'));
 assert.equal(final.fields.channel,undefined);
 assert.equal(final.fields.generate_audio,undefined);
});
