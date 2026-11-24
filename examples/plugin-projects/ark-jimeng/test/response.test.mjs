import test from 'node:test';import assert from 'node:assert/strict';import {channelResult} from '../dist/response.mjs';
test('only the original completed channel and actual media category can proceed',()=>{
 const good={id:'original',state:'done',route:'api',media:[{kind:'video',url:'https://example.com/original.mp4'}],draft:null};
 assert.equal(channelResult(good,'original','video'),good);
 for(const bad of [{...good,id:'replacement'},{...good,state:'running'},{...good,route:'fallback'},{...good,media:[{kind:'image',url:good.media[0].url}]},{...good,media:[{kind:'video',url:'http://127.0.0.1/private'}]}])assert.throws(()=>channelResult(bad,'original','video'));
});
