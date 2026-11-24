import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';

test('one visible MiniMax entry per model prefers Design while legacy models and voice identities remain compatible',async()=>{
 const release=JSON.parse(await readFile(new URL('../release.template.json',import.meta.url),'utf8'));
 for(const model of release.generation.models){
  if(model.hidden===true)continue;
  assert.ok(['auto','subscription'].includes(model.fields.access.default),model.id+' bypasses Design by default');
 }
 const h3=release.generation.models.find(m=>m.id==='MiniMax-H3-auto');
 assert.equal(h3.fields.access.default,'auto');
 assert.equal(h3.fields.access.enum.includes('api'),true);
 assert.equal(h3.hidden,false);
 assert.equal(release.generation.models.find(m=>m.id==='MiniMax-H3').hidden,true);
 assert.equal(release.generation.models.find(m=>m.id==='MiniMax-H3').fields.access.default,'api');
 const visible=release.generation.models.filter(m=>!m.hidden);
 assert.equal(new Set(visible.map(m=>m.id.replace(/-auto$/,''))).size,visible.length);
 assert.equal(visible.some(m=>m.label.includes('订阅优先')),false);
 assert.equal(release.generation.name,'MiniMax');
 assert.ok(release.compatibility.channel.capabilities.some(c=>c.id==='channel.design-audio-reference'&&c.version===1));
});

test('only the standard H3 reference mode exposes the video port and requests protected video URLs',async()=>{
 const release=JSON.parse(await readFile(new URL('../release.template.json',import.meta.url),'utf8'));
 const standard=release.generation.models.filter(m=>['MiniMax-H3','MiniMax-H3-auto'].includes(m.id));
 assert.equal(standard.length,2);
 for(const model of standard){
  assert.ok(model.inputKinds.includes('video'));
  assert.equal(model.maxInputs,12);
  for(const [mode,details]of Object.entries(model.fields.input_mode.inputModes))assert.equal(details.videos.max,mode==='reference'?3:0,mode);
 }
 assert.ok(release.generation.models.filter(m=>m.id!=='MiniMax-H3-Regeneration'&&!standard.includes(m)).every(m=>!m.inputKinds.includes('video')));
 const policy=JSON.parse(await readFile(new URL('../fixtures/policies/generate.json',import.meta.url),'utf8'));
 for(const key of ['video','videoURLs'])assert.deepEqual(policy.body.inputs[key],{$host:'inputs',kind:'video',encoding:'public-url'});
});

test('synthetic channel probes meet the installed connection negotiation requirements',async()=>{
 const connection=JSON.parse(await readFile(new URL('../fixtures/connection.json',import.meta.url),'utf8'));
 const fixture=JSON.parse(await readFile(new URL('../fixtures/verification.json',import.meta.url),'utf8'));
 const required=connection.negotiation.required;
 const probes=fixture.exchanges.filter(exchange=>exchange.channelProbe);
 assert.ok(probes.length>0);
 for(const probe of probes){
  const advertised=JSON.parse(Buffer.from(probe.body,'base64').toString('utf8'));
  assert.ok(advertised.implementationVersion.localeCompare(required.minimumVersion,'en',{numeric:true})>=0,probe.callId);
  assert.equal(advertised.version,required.version,probe.callId);
  for(const capability of required.capabilities)assert.ok(advertised.capabilities.some(offered=>offered.id===capability.id&&offered.version===capability.version),probe.callId+': '+capability.id);
 }
});
