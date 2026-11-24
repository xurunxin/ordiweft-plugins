import test from 'node:test';
import assert from 'node:assert/strict';
import {channelResult} from '../dist/response.mjs';
import {validateInputs} from '../dist/preflight.mjs';
test('result is bound to original ID, route and media kind including inline audio',()=>{
 const value={id:'original',state:'done',route:'subscription',media:[{kind:'audio',data:'UklGRg==',voice:{provider:'minimax',id:'voice'}}]};
 assert.equal(channelResult(value,'original','audio').media[0].voice.id,'voice');
 for(const candidate of [{...value,id:'replacement'},{...value,route:'cli'},{...value,media:[{kind:'audio',url:'http://host/private'}]},{...value,media:[{kind:'audio',data:'UklGRg==',url:'https://media.example/a'}]}])assert.throws(()=>channelResult(candidate,'original','audio'));
});

test('subscription HTTP 451 is classified only for the bound original refusal',()=>{
 const refusal={id:'original',state:'failed',route:'subscription',submission:'unknown',errorCode:'subscription_provider_http_451'};
 for (const submission of ['unknown','accepted']) {
  assert.throws(()=>channelResult({...refusal,submission,media:[]},'original','video'), {message:'minimax_prompt_content_filter'});
 }
 for (const change of [
  {id:'other'}, {route:'api'}, {state:'running'}, {submission:'rejected'},
  {errorCode:'subscription_provider_rejected'}, {media:'invalid'}, {media:[{}]}, {draft:{}},
 ]) {
  assert.throws(()=>channelResult({...refusal,...change},'original','video'), {message:'Original result unconfirmed'});
 }
});
test('image references use host byte facts and mode cardinality before submitting',()=>{
 const input={kind:'image',asset:{bytes:100,sha256:'a'.repeat(64)}},fact={bytes:100,sha256:'a'.repeat(64),mime:'image/png',width:512,height:512};
 const generation={capability:'video.generate',parameters:{input_mode:'first_frame'}};
 validateInputs(generation,[input],[fact]);
 for(const [inputs,facts]of [[[],[]],[[input],[{...fact,width:128}]],[[input],[{...fact,sha256:'b'.repeat(64)}]],[[{...input,kind:'audio'}],[fact]]])assert.throws(()=>validateInputs(generation,inputs,facts));
 assert.throws(()=>validateInputs({capability:'speech.generate',parameters:{}},[input],[fact]));
});

test('standard H3 accepts bounded original voice audio only in multimodal reference mode',()=>{
 const image={kind:'image',asset:{bytes:100,sha256:'a'.repeat(64)}},imageFact={...image.asset,mime:'image/png',width:512,height:512};
 const audio={kind:'audio',asset:{bytes:200,sha256:'b'.repeat(64)}},audioFact={...audio.asset,mime:'audio/mpeg',durationMs:5000};
 const generation={model:'MiniMax-H3-auto',capability:'video.generate',parameters:{input_mode:'reference'}};
 validateInputs(generation,[image,audio],[imageFact,audioFact]);
 validateInputs(generation,[audio,audio,audio],[audioFact,audioFact,audioFact]);
 for(const [patch,inputs,facts]of [
  [{model:'MiniMax-H3-Lite'},[audio],[audioFact]],
  [{parameters:{input_mode:'first_frame'}},[image,audio],[imageFact,audioFact]],
  [{},[audio],[{...audioFact,durationMs:1999}]],
  [{},[audio],[{...audioFact,mime:'audio/ogg'}]],
  [{},[audio],[{...audioFact,sha256:'c'.repeat(64)}]],
  [{},[audio,audio],[{...audioFact,durationMs:8000},{...audioFact,durationMs:8000}]],
  [{},Array(4).fill(audio),Array(4).fill({...audioFact,durationMs:2000})],
  [{},Array(10).fill(image),Array(10).fill(imageFact)],
 ])assert.throws(()=>validateInputs({...generation,...patch},inputs,facts));
});

test('standard H3 accepts bounded reference videos on all supported access routes',()=>{
 const clip={kind:'video',asset:{bytes:50000000,sha256:'c'.repeat(64)}};
 const facts={...clip.asset,mime:'video/mp4',width:1920,height:1080,durationMs:5000};
 const mode={input_mode:'reference',access:'api'};
 const generation={model:'MiniMax-H3-auto',capability:'video.generate',parameters:mode};
 validateInputs(generation,[clip],[facts]);
 validateInputs(generation,Array(3).fill(clip),Array(3).fill(facts));
 validateInputs({...generation,model:'MiniMax-H3',parameters:{input_mode:'reference'}},[clip],[facts]);
 for(const access of ['auto','subscription',undefined])validateInputs({...generation,parameters:{...mode,access}},[clip],[facts]);
 for(const input of [
  [{...generation,model:'MiniMax-H3-Lite-auto'},[clip],[facts]],
  [{...generation,parameters:{...mode,input_mode:'first_last_frame'}},[clip],[facts]],
  [generation,[clip],[{...facts,mime:'video/webm'}]],
  [generation,[clip],[{...facts,durationMs:1999}]],
  [generation,[clip],[{...facts,durationMs:15001}]],
  [generation,[clip],[{...facts,height:255}]],
  [generation,[clip],[{...facts,width:300,height:1000}]],
  [generation,[clip],[{...facts,sha256:'d'.repeat(64)}]],
  [generation,[{...clip,asset:{...clip.asset,bytes:50000001}}],[{...facts,bytes:50000001}]],
  [generation,Array(4).fill(clip),Array(4).fill({...facts,durationMs:2000})],
  [generation,[clip,clip],[{...facts,durationMs:10001},facts]],
 ])assert.throws(()=>validateInputs(...input));
});
