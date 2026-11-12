import {openAction} from './host.mjs'
const task=await openAction(process.stdin,process.stdout)
if(task.portId!=='document-text'||task.portKind!=='processor'||task.inputs.length)throw Error('Unsupported action')
const {title,paragraphs}=task.arguments
const text=[title,...paragraphs].join('\n\n')
await task.complete([],{text,paragraphCount:paragraphs.length,characters:Array.from(text).length})
