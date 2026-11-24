import {openAction} from './host.mjs'

const task = await openAction(process.stdin, process.stdout)
if (task.inputs.length !== 1) throw new Error('one_input_required')
const input = task.inputs[0]
const info = await task.host.call('inspect-original', 'media', 'inspect', {id: input.asset.id})
const prefix = await task.host.call('read-original-prefix', 'assets', 'read', {id: input.asset.id, offset: 0, limit: 8})
await task.complete([], {kind: input.kind, mime: info.mime, sha256: info.sha256, bytes: info.bytes, width: info.width ?? 0, height: info.height ?? 0, durationMs: info.durationMs ?? 0, signature: Buffer.from(prefix.data, 'base64').toString('hex')})
