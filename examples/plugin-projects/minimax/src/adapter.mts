import {openTask,readAsset,type AssetHandle,type PrivateReadable,type PrivateWritable} from '@ordiweft/plugin-host';
import {channelResult} from './response.mjs';
import {validateInputs} from './preflight.mjs';
declare const process:{stdin:PrivateReadable;stdout:PrivateWritable};
const task=await openTask(process.stdin,process.stdout);
const facts=[];
for(const [i,input]of task.inputs.entries())facts.push(await task.host.call('original-inspect-'+i,'media','inspect',{id:input.asset.id}));
validateInputs(task.generation,task.inputs,facts);
type Response={status:number;body:AssetHandle};
const submitted=await task.host.call('original-submit','connections','request',{operation:'generate'}) as unknown as Response;
if(submitted.status!==202)throw Error('Original acceptance unconfirmed');
const original=JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(await readAsset(task.host,'submit-body',submitted.body,65536))) as {id:string};
if(typeof original.id!=='string'||!/^[A-Za-z0-9_-]{1,128}$/.test(original.id))throw Error('Original identity missing');
await task.host.call('original-checkpoint','jobs','checkpoint',{metadata:{originalCall:'original-submit'}});
const waited=await task.host.call('original-observation','jobs','wait',{name:'result'}) as unknown as {response:Response;receipt:{callId:string}};
if(waited.response.status!==200)throw Error('Original observation unconfirmed');
const observed=await task.host.call('original-result','connections','request',{operation:'result'}) as unknown as Response;
if(observed.status!==200)throw Error('Original result unconfirmed');
const body=await readAsset(task.host,'result-body',observed.body,67108864);
const expected=task.generation.capability==='speech.transcribe'?'transcript':task.generation.capability==='video.generate'?'video':task.generation.capability==='image.generate'?'image':'audio';
const result=channelResult(JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(body)),original.id,expected);
const outputs:string[]=[];
for(const [i,media]of result.media.entries()){
 const decoded=media.data!==undefined
  ?await task.host.call('original-decode-'+i,'assets','decode',{id:observed.body.id,encoding:'json-base64',pointer:'/media/'+i+'/data',maxBytes:41943040,maxSourceBytes:67108864})
  :await task.host.call('original-download-'+i,'transfers','download',{name:'media-'+i,callId:'original-result'});
 const asset=(decoded as unknown as {asset:AssetHandle}).asset;
 const staged=await task.host.call('original-stage-'+i,'outputs','stage',{id:asset.id,...media.voice?{voice:media.voice}:{}}) as unknown as {output:AssetHandle};
 outputs.push(staged.output.id);
 await task.host.call('original-release-'+i,'assets','release',{id:asset.id});
}
await task.host.call('release-observation','assets','release',{id:waited.response.body.id});
await task.host.call('release-result','assets','release',{id:observed.body.id});
await task.host.call('release-submit','assets','release',{id:submitted.body.id});
await task.complete(outputs,{...result.metadata,channel:result.route,channelJob:result.id,transport:'host-services'});
