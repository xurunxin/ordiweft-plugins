// Offline SDK v2 example: emits a fixed one-pixel PNG, no network or credentials.
import fs from 'node:fs'
const call=JSON.parse(fs.readFileSync(0,'utf8'))
if(call.apiVersion!==2||call.method!=='execute'||call.request?.secrets&&Object.keys(call.request.secrets).length)throw Error('unsupported call')
const generation=call.request.payload
if(generation.provider!=='isolated-image'||generation.model!=='pixel'||generation.capability!=='image.generate')throw Error('unsupported generation')
const data='iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jXx8AAAAASUVORK5CYII='
process.stdout.write(JSON.stringify({apiVersion:2,kind:'result',result:{media:[{data}],metadata:{fixture:true,network:'none'}}}))
