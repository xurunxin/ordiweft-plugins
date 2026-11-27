// SPDX-License-Identifier: Apache-2.0

const source = 'https://apimart.ai/zh/pricing'
const tokenUnits = {
  input: 'input_token', text_input: 'input_text_token', image_input: 'input_image_token',
  output: 'output_token',
  cached_input: 'cache_read_token', cache_write: 'cache_write_token',
}

function component(unit, rate, path, per = 1, multiplier = 1) {
  if (!Number.isFinite(rate) || rate < 0) return undefined
  return {unit, rate: (rate * multiplier).toFixed(9).replace(/\.?0+$/, ''), per: String(per),
    dimension: path && ['apimart', ...path.map(encodeURIComponent), ...(multiplier === 1 ? [] : ['*' + multiplier])].join(':'), catalogPer: path && String(per)}
}

export function modelPricing(entry, row, checkedAt) {
  const id = entry.upstream || entry.model.id
  const spec = {provider: 'apimart', model: id, currency: 'USD', sourceUrl: source, checkedAt}
  if (!row) return spec
  const {fields, capability} = entry.model
  const rules = new Map()
  const values = (key, val) => fields[key]?.enum?.filter(v => String(v).toLowerCase() === String(val).toLowerCase()) ?? []
  const add = (when, ...components) => {
    if (components.length && components.every(Boolean)) rules.set(JSON.stringify(when), {when, components})
  }
  const billingUnit = row.billing_type === 'per_second' ? 'video_second' : capability === 'image.generate' ? 'image' : 'job'
  const use = (key, value, ...paths) => component(billingUnit, value, [key, ...paths])
  const spell = (key, raw) => values(key, raw)[0]
  const perInputImage = row.input_image_first_free === false
    ? component('input_image', row.input_image_paid_price, ['input_image_paid_price']) : undefined
  const imageCharges = part => perInputImage && entry.model.inputKinds.includes('image') ? [part, perInputImage] : [part]
  if (entry.model.id === 'MiniMax-H3-Regeneration') {
    const rate = row.resolution_paid_prices?.['2K']
    const output = component('video_second', rate, ['resolution_paid_prices', '2K'])
    const input = component('input_video_second', rate, ['resolution_paid_prices', '2K'])
    const excessImage = component('job', row.input_image_paid_price, ['input_image_paid_price'])
    for (let videos = 0; videos <= 3; videos++) {
      const timeCharges = videos === 0 ? [output] : [output, input]
      for (let images = 0; images <= 9; images++) {
        add({reference_image_count: String(images), reference_video_count: String(videos)},
          ...timeCharges, ...Array(Math.max(0, images - 5)).fill(excessImage))
      }
    }
  } else if (entry.model.id === 'seedance-2.5-final-1080p') {
    for (const resolution of fields.resolution.enum) {
      for (let videos = 0; videos <= 10; videos++) {
        const key = 'token-' + resolution.toUpperCase() + (videos === 0 ? '' : '-input')
        add({resolution, reference_video_count: String(videos)},
          component('output_token', row.billing_tier_paid_prices?.[key], ['billing_tier_paid_prices', key], 1000000))
      }
    }
  } else if (id === 'flux-3-video') {
    const draft = entry.fixed?.draft === true
    const continuation = Boolean(entry.model.taskContinuation)
    const videoInput = entry.references.some(r => r.kind === 'video')
    const resolutions = draft ? ['hd'] : fields.resolution.enum
    for (const resolution of resolutions) {
      for (const videos of continuation ? [0,1] : [Number(videoInput)]) {
        const key = (videos ? 'V2V-' : '') + (draft ? 'DRAFT' : resolution.toUpperCase())
        const when = draft ? {} : {resolution}
        if (continuation) when.reference_video_count = String(videos)
        add(when, use('resolution_paid_prices', row.resolution_paid_prices?.[key], key))
      }
    }
  } else if (id === 'apimart-zoe') {
    const draft = entry.fixed?.draft === true
    const final = Boolean(entry.model.taskContinuation)
    const maxVideos = final ? 10 : entry.references.find(r => r.kind === 'video')?.max ?? 0
    const resolutions = draft || final ? [entry.fixed.resolution] : fields.resolution.enum
    for (const resolution of resolutions) {
      for (let videos = 0; videos <= maxVideos; videos++) {
        const key = resolution.toUpperCase() + (videos ? '-input' : '')
        const rate = row.resolution_paid_prices?.[key]
        const path = ['resolution_paid_prices', key]
        const when = (draft || final) ? {} : {resolution}
        when.reference_video_count = String(videos)
        add(when, component('video_second', rate, path), ...(videos ? [component('input_video_second', rate, path)] : []))
      }
    }
  } else if (entry.fixed?.draft === true) {
    const maxVideos = entry.references.find(r => r.kind === 'video')?.max ?? 0
    for (let videos = 0; videos <= maxVideos; videos++) {
      const key = videos === 0 ? '480P' : '480P-input'
      const rate = row.resolution_paid_prices?.[key]
      const path = ['resolution_paid_prices', key]
      const input = videos === 0 ? [] : [component('input_video_second', rate, path)]
      add({reference_video_count: String(videos)}, component('video_second', rate, path), ...input)
    }
  } else if (row.billing_type === 'tiered_token') {
    const rates = row.pricing?.effective_rates
    if (rates && row.pricing.unit === 'usd_per_million_tokens' && row.pricing.tier_count === 1) {
      const positive = Object.entries(rates).filter(([key, rate]) => rate > 0 &&
        !(capability === 'speech.generate' && key === 'audio_output' && rates.output === rate))
      if (positive.length && positive.every(([key]) => tokenUnits[key])) {
        add({}, ...positive.map(([key, rate]) => component(tokenUnits[key], rate, ['pricing', 'effective_rates', key], 1000000)))
      }
    }
  } else if (row.billing_type === 'suno_action' && entry.action) {
    const byVersion = fields.version?.enum || []
    for (const v of byVersion) {
      const key = `suno@${entry.action}-${v}`
      add({version: v}, component('job', row.action_paid_prices?.[key], ['action_paid_prices', key]))
      if (fields.max_mode) add({version: v, max_mode: 'true'}, component('job', row.action_paid_prices?.[key], ['action_paid_prices', key], 1, row.max_mode_multiplier || 2))
    }
    if (!byVersion.length) {
      const key = 'suno@' + entry.action
      add({}, component('job', row.action_paid_prices?.[key], ['action_paid_prices', key]))
    }
  } else if (row.billing_type === 'per_minute') {
    add({}, component('audio_second', row.price_per_minute, ['price_per_minute'], 60))
  } else if (row.billing_type === 'mj_action') {
    const operation = entry.action || 'imagine'
    for (const speed of fields.speed?.enum ?? []) {
      add({speed,...(fields.repeat ? {repeat:'1'} : {})}, component('job', row.action_paid_prices?.[operation]?.[speed], ['action_paid_prices', operation, speed]))
    }
    if (!fields.speed) add({}, component('job', row.action_paid_prices?.[operation]?.default, ['action_paid_prices', operation, 'default']))
  } else if (row.billing_type === 'version_resolution') {
    for (const [version, matrix] of Object.entries(row.version_resolution_paid_prices ?? {})) {
      for (const [resolution, rate] of Object.entries(matrix)) {
        const selection = spell('resolution', resolution)
        if (selection && spell('version', version)) add({version, resolution: selection}, use('version_resolution_paid_prices', rate, version, resolution))
      }
    }
  } else if (row.billing_type === 'resolution_duration') {
    for (const [key, rate] of Object.entries(row.resolution_duration_paid_prices ?? {})) {
      const [resolution, seconds] = key.split('-')
      const selection = spell('resolution', resolution)
      if (selection && spell('duration', parseInt(seconds))) add({resolution: selection, duration: String(parseInt(seconds))}, component('job', rate, ['resolution_duration_paid_prices', key]))
    }
  } else if (row.billing_type === 'size_quality') {
    for (const [key, matrix] of Object.entries(row.size_quality_paid_prices ?? {})) {
      const resolution = spell('resolution', key)
      if (!resolution) continue
      for (const [quality, rate] of Object.entries(matrix)) {
        if (spell('quality', quality)) add({resolution, quality}, ...imageCharges(use('size_quality_paid_prices', rate, key, quality)))
      }
    }
  } else {
    const actionKey = entry.action && id + '@' + entry.action
    if (actionKey && row.action_paid_prices?.[actionKey] != null) {
      add({}, component('job', row.action_paid_prices[actionKey], ['action_paid_prices', actionKey]))
    } else {
      const tiers = Object.entries(row.resolution_paid_prices ?? {})
      const dimension = ['resolution', 'size', 'mode'].find(field => tiers.some(([key]) => spell(field, key)))
      for (const [key, rate] of tiers) {
        const selection = dimension && spell(dimension, key)
        if (!selection) continue
        const base = use('resolution_paid_prices', rate, key)
        if (row.resolution_paid_prices[key + '-input'] == null || !entry.model.inputKinds.includes('video')) {
          add({[dimension]: selection}, ...imageCharges(base))
        } else {
          add({[dimension]: selection, reference_video_count: '0'}, base)
          const videoRefs = entry.references.filter(r => r.kind === 'video').reduce((sum, r) => sum + r.max, 0)
          for (let n = 1; n <= videoRefs; n++) {
            add({[dimension]: selection, reference_video_count: String(n)}, use('resolution_paid_prices', row.resolution_paid_prices[key + '-input'], key + '-input'))
          }
        }
        if (fields.layer_decomposition && row.layer_decomposition_paid_prices?.[key] != null) {
          add({[dimension]: selection, layer_decomposition: 'false'}, ...imageCharges(base))
          add({[dimension]: selection, layer_decomposition: 'true'}, ...imageCharges(use('layer_decomposition_paid_prices', row.layer_decomposition_paid_prices[key], key)))
          rules.delete(JSON.stringify({[dimension]: selection}))
        }
      }
      if (!tiers.length && fields.mode?.enum && row.billing_tier_paid_prices && !row.billing_matrix_paid_prices) {
        for (const mode of fields.mode.enum) {
          const silent = mode === 'std' ? 'default' : mode
          const sound = mode === 'std' ? 'sound' : mode + '-sound'
          const modePrice = silent === 'default' ? use('paid_price', row.paid_price) : use('billing_tier_paid_prices', row.billing_tier_paid_prices[silent], silent)
          add(fields.audio ? {mode, audio: 'false'} : {mode}, modePrice)
          if (fields.audio) add({mode, audio: 'true'}, use('billing_tier_paid_prices', row.billing_tier_paid_prices[sound], sound))
        }
      }
      if (!tiers.length && !row.billing_tier_paid_prices && !row.billing_matrix_paid_prices) add({}, ...imageCharges(use('paid_price', row.paid_price)))
      if (!dimension && !row.billing_matrix_paid_prices && !row.billing_tier_paid_prices) {
        const fixedResolution = String(entry.fixed?.resolution ?? '').toUpperCase()
        const fixedRate = row.resolution_paid_prices?.[fixedResolution]
        const withVideo = row.resolution_paid_prices?.[fixedResolution + '-input']
        if (fixedRate != null && withVideo == null) add({}, use('resolution_paid_prices', fixedRate, fixedResolution))
        if (fixedRate != null && withVideo != null) {
          const maxVideo = entry.references.filter(r => r.kind === 'video').reduce((sum, r) => sum + r.max, 0)
          if (maxVideo === 0) add({}, use('resolution_paid_prices', fixedRate, fixedResolution))
          else {
            add({reference_video_count:'0'},use('resolution_paid_prices', fixedRate, fixedResolution))
            for (let n = 1; n <= maxVideo; n++) {
              add({reference_video_count:String(n)},use('resolution_paid_prices',withVideo,fixedResolution+'-input'))
            }
          }
        }
      }
      if (id === 'kling-v4-long-image') {
        for (const [key, rate] of tiers) {
          const [resolution, type] = key.toLowerCase().split('-')
          const selection = spell('resolution', resolution)
          if (!selection) continue
          const components = imageCharges(use('resolution_paid_prices',rate,key))
          if (type==='webpage') add({resolution:selection,skill:'webpage'},...components)
          else if (spell('length',type)) add({resolution:selection,skill:'ecommerce',length:type},...components)
        }
      }
    }
  }
  return {...spec, rules: [...rules.values()].map(rule => fields.official_fallback ? {...rule,when:{...rule.when,official_fallback:"false"}} : rule)}
}
