// SPDX-License-Identifier: Apache-2.0
import { readFile, writeFile } from 'node:fs/promises'
import { entries } from './definitions.mjs'
import { modelPricing } from './pricing.mjs'
import './image-models.mjs'
import './video-models.mjs'
import './audio-models.mjs'

const [pricesFile, checkedAt, output] = process.argv.slice(2)
if (!pricesFile || !/^\d{4}-\d{2}-\d{2}$/.test(checkedAt) || !output) {
  throw new Error('Usage: node build-catalog.mjs <public-prices.json> <YYYY-MM-DD> <new-catalog.json>')
}
const prices = JSON.parse(await readFile(pricesFile, 'utf8'))
const records = entries.map(entry => {
  const price = modelPricing(entry, prices[entry.upstream || entry.model.id], checkedAt)
  const {action, ...spec} = entry
  return {...spec, model: {...spec.model, pricing: price}}
})
await writeFile(output, records.map(record => JSON.stringify(record)).join(',\n').replace(/^/, '[\n') + '\n]\n', {flag: 'wx'})
