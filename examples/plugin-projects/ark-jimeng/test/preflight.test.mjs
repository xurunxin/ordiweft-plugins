import test from 'node:test';
import assert from 'node:assert/strict';
import {validateInputs} from '../dist/preflight.mjs';
const input={kind:'image',role:'reference',asset:{id:'original',sha256:'a'.repeat(64),bytes:100}};
const image={sha256:input.asset.sha256,bytes:100,mime:'image/png',width:512,height:512,transparent:true};
const generation=(parameters={},prompt='fixture')=>({model:'doubao-seedream-5-0-pro-260628',capability:'image.generate',prompt,parameters});
test('image mode rules reject before submitting while layers allow blank text',()=>{
 assert.throws(()=>validateInputs(generation({},''),[],[]),/描述/);
 assert.doesNotThrow(()=>validateInputs(generation({layer_decomposition:true},''),[input],[image]));
 assert.throws(()=>validateInputs(generation({layer_decomposition:true},''),[],[]),/拆层/);
 for(const transparent of [false,undefined])assert.throws(()=>validateInputs(generation({background:'transparent',output_format:'png'}),[input],[{...image,transparent}]),/透明/);
 assert.doesNotThrow(()=>validateInputs(generation({background:'transparent',output_format:'png'}),[input],[image]));
 assert.throws(()=>validateInputs(generation({sequential_image_generation_options:{max_images:15}}),[input],[image]),/15/);
 assert.throws(()=>validateInputs(generation(),[input],[{...image,sha256:'b'.repeat(64)}]),/changed/);
});
test('original video facts enforce actual dimensions, per-input and total durations',()=>{
 const g={model:'doubao-seedance-2-5-260628',capability:'video.generate',prompt:'fixture',parameters:{input_mode:'reference'}};
 const video={...input,kind:'video'},facts={...image,mime:'video/mp4',width:1280,height:720,durationMs:16000};
 assert.doesNotThrow(()=>validateInputs(g,[video],[facts]));
 assert.throws(()=>validateInputs(g,[video,video],[facts,facts]),/总时长/);
 assert.throws(()=>validateInputs(g,[video],[{...facts,width:64}]),/尺寸/);
 assert.throws(()=>validateInputs({...g,parameters:{sourceJob:'original'}},[video],[facts]),/原样片/);
});

test('Seedance 2.0 and 2.5 retain actual audio references with their different audio-only limits',()=>{
 const audio={kind:'audio',role:'reference_audio',asset:{id:'original-audio',sha256:'b'.repeat(64),bytes:200}};
 const facts={sha256:audio.asset.sha256,bytes:200,mime:'audio/wav',durationMs:3000};
 for(const model of ['doubao-seedance-2-0-260128','doubao-seedance-2-0-fast-260128','doubao-seedance-2-0-mini-260615','doubao-seedance-2-5-260628','seedance-2.5-auto']){
  const g={model,capability:'video.generate',prompt:'Use the original voice reference',parameters:{input_mode:'reference'}};
  assert.doesNotThrow(()=>validateInputs(g,[input,audio],[image,facts]));
  assert.throws(()=>validateInputs(g,[input,audio],[image,{...facts,sha256:'c'.repeat(64)}]),/changed/);
  if(model.includes('2-0-'))assert.throws(()=>validateInputs(g,[audio],[facts]),/不支持纯音频/);
  else assert.doesNotThrow(()=>validateInputs(g,[audio],[{...facts,durationMs:20000}]));
 }
});
