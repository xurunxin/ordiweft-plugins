import {createHash} from 'node:crypto';
import {openAction} from './host.mjs';
const task=await openAction(process.stdin,process.stdout);
const {host}=task;
const {title,paragraphs}=task.arguments;
const text=[title,...paragraphs].filter(Boolean).join('\n\n');
const bytes=Buffer.from(text,'utf8');
const sha256=createHash('sha256').update(bytes).digest('hex');
const buffer=await host.call('document-file','outputs','begin',{bytes:bytes.length,sha256});
for(let offset=0;offset<bytes.length;offset+=65536){
  await host.call('document-chunk-'+offset,'outputs','write',{id:buffer.id,offset,data:bytes.subarray(offset,offset+65536).toString('base64')});
}
const result=await host.call('document-stage','outputs','stage',{id:buffer.id});
await task.complete([result.output.id],{characters:Array.from(text).length,paragraphCount:paragraphs.length});
