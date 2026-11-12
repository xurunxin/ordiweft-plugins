import {openAction} from './host.mjs'
const task=await openAction(process.stdin,process.stdout)
if(task.portId!=='analyze-text'||task.portKind!=='processor'||task.inputs.length)throw Error('Unsupported action')
const text=task.arguments.text
await task.complete([],{characters:Array.from(text).length,words:text.trim()?text.trim().split(/\s+/u).length:0})
