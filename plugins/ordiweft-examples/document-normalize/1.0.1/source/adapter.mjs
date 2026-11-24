import {openAction,readAsset,stageOutput} from './host.mjs';
const task=await openAction(process.stdin,process.stdout);
if(task.inputs.length!==1||task.inputs[0].kind!=='document'||task.inputs[0].role!=='source'||task.inputs[0].asset.mime!=='text/plain')throw Error('Expected one UTF-8 source document');
const input=task.inputs[0].asset;
const original=await readAsset(task.host,'original',input,1048576);
const text=Buffer.from(original).toString('utf8').replace(/\r\n?/g,'\n');
const output=await stageOutput(task.host,'normalized',Buffer.from(text,'utf8'),4194304);
await task.complete([output.id],{characters:Array.from(text).length,lines:text.split('\n').length,sourceSha256:input.sha256});
