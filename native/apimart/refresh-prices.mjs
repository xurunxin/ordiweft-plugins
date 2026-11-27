// SPDX-License-Identifier: Apache-2.0
import { writeFile } from 'node:fs/promises'
import { entries } from './definitions.mjs'
import './image-models.mjs'
import './video-models.mjs'
import './audio-models.mjs'

const [output] = process.argv.slice(2)
if (!output) throw new Error('Usage: node refresh-prices.mjs <new-public-prices.json>')
const ids = [...new Set(entries.map(entry => entry.upstream || entry.model.id))]
const prices = new Map()
const unavailable = []
const queue = ids.slice()
await Promise.all(Array.from({length: Math.min(8, queue.length)}, async () => {
  while (queue.length) {
    const id = queue.shift()
    try {
      const url = new URL('https://api.apimart.ai/api/pricing/model')
      url.searchParams.set('model', id)
      const response = await fetch(url, {signal: AbortSignal.timeout(10000), redirect: 'error'})
      const body = await response.json()
      if (!response.ok || !body.success || body.data?.model_name !== id) throw new Error('invalid public tariff')
      prices.set(id, body.data)
    } catch {
      unavailable.push(id)
    }
  }
}))
if (unavailable.length) throw new Error('Missing APIMart prices: ' + unavailable.sort().join(', '))
await writeFile(output, JSON.stringify(Object.fromEntries(ids.map(id => [id, prices.get(id)])), null, 2) + '\n', {flag: 'wx'})
