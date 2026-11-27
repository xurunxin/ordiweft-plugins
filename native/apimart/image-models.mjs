// SPDX-License-Identifier: Apache-2.0
import { add, decimal, flag, generationMode, imageAndText, images, line, number, object, options, ratios, ref, wideRatios } from './definitions.mjs'

const gptRatios = ratios + ' 2:1 1:2 9:21 3:1 1:3'
const ratios7 = '1:1 16:9 9:16 4:3 3:4 3:2 2:3'
const ratio = (choices = ratios, value = '1:1') => options(choices, value)
const res = (choices = '1K 2K 4K', value = '1K') => options(choices, value)
const max32 = 4294967295
const seed = number(0, max32)
const n = max => number(1, max, 1)
const sizeNote = '也可以输入文档允许的像素尺寸，例如 2048x2048。'
const pixels = line(24, {label: '画面尺寸', description: sizeNote})
const masked = max => [...images(max), ref('mask_url','image',1,{role:'mask',inline:true})]
const format = (choices = 'jpeg png webp', value = 'png') => options(choices, value)
const addImage = (ids, fields, refs, settings = {}) => add('image', ids, fields, refs, {
  ...(refs.length && !refs.some(ref => ref.min>0)?imageAndText():{}),...settings,
})
const countGroup = (max, mode = 'sequential_image_generation') => object({
  max_images: number(1, max, undefined, {label: '最大图片数', required: true}),
}, {label: '组图数量', visibleWhen: {[mode]: 'auto'}})

addImage( 'dall-e-3', {
  size: options('1024x1024 1792x1024 1024x1792', '1024x1024'), quality: options('standard hd', 'standard'),
}, [], {name: 'DALL·E 3', maxPrompt: 4000, fixed: {n: 1}})

const nanoLabels = {
  'gemini-2.5-flash-image-preview': 'Nano banana · Ext',
  'gemini-2.5-flash-image-preview-official': 'Nano banana · 官方版',
  'gemini-3-pro-image-preview': 'Nano banana Pro · Ext',
  'gemini-3-pro-image-preview-official': 'Nano banana Pro · 官方版',
  'gemini-3.1-flash-image-preview': 'Nano banana 2 · Ext',
  'gemini-3.1-flash-image-preview-official': 'Nano banana 2 · 官方版',
  'gemini-3.1-flash-lite-image': 'Nano banana 2 Lite · 官方版',
  'gemini-3.1-flash-lite-image-ext': 'Nano banana 2 Lite · Ext',
}
for (const [id, name] of Object.entries(nanoLabels)) {
  const v31 = id.startsWith('gemini-3.1')
  const lite = id.includes('lite')
  const official = id.endsWith('official') || id === 'gemini-3.1-flash-lite-image'
  addImage( id, {
    size: ratio('auto ' + (v31 ? wideRatios : ratios), 'auto'),
    ...(v31 && !lite ? {resolution: res('0.5K 1K 2K 4K')} : id.startsWith('gemini-3-pro') ? {resolution: res()} : {}),
    ...(official && !lite ? {n: n(4)} : {}),
    ...(v31 && !lite ? {google_search: flag(false), google_image_search: flag(false)} : {}),
  }, images(id.startsWith('gemini-2.5') ? 3 : 14), {name, fixed: lite || !official ? {n: 1} : undefined})
}

const commonGPT = {
  quality: options('auto low medium high','auto'),
  output_format: format(),
  output_compression: number(0, 100, undefined, {visibleWhenAny: [{output_format: 'jpeg'}, {output_format: 'webp'}]}),
  moderation: options('auto low','auto'), n: n(4),
}
addImage('gpt-image-1 gpt-image-1-official gpt-image-1.5 gpt-image-1.5-official', {
  ...commonGPT, size: ratio('1:1 3:2 2:3'),
  background: options('auto opaque transparent','auto'),
}, masked(15), {...imageAndText('参考图编辑'),usage: '可连入参考图及透明 PNG 蒙版。压缩质量只在 JPEG 和 WebP 下可设置。'})
addImage('gpt-image-2', {size:ratio(gptRatios), resolution:res('1k 2k 4k','1k'), n:n(4)}, images(15))
addImage('gpt-image-2-official', {...commonGPT, size:ratio(gptRatios), resolution:res('1k 2k 4k','1k')}, masked(16))
const nextGPT = {...commonGPT, quality: options('auto low medium high xhigh max','auto'), resolution: res('1k 2k 4k','1k'),
  size: pixels, background: options('auto opaque transparent','auto')}
addImage( 'gpt-image-2.5-flare gpt-image-2.5-sunburst', nextGPT, masked(16))
addImage( 'gpt-image-2.5-ext', {version: options('flare sunburst','flare'), size: ratio('auto ' + ratios, 'auto'), resolution: res(), n: n(4)}, images(16))
const gif = {output_format:'gif'}
addImage( 'gpt-image-2.6-flare gpt-image-2.6-sunburst', {
  ...nextGPT, output_format: format('png jpeg webp gif svg'),
  frame_count: number(2, 16, 9, {label:'帧数', visibleWhen:gif}),
  frame_duration_ms: number(20, undefined, 200, {label:'每帧时长（毫秒）', step:10, visibleWhen:gif}),
}, masked(16), {usage: '静态图可使用参考图与蒙版。GIF 尺寸为单帧像素尺寸，不使用比例名或蒙版。'})
addImage( 'gpt-image-2.6-ext', {
  version:options('flare sunburst','flare'), size:ratio('auto '+ratios,'auto'), resolution: res('1k 2k 4k','1k'), n:n(4),
  output_format:format('png jpeg webp gif svg'), frame_count:number(2,16,9,{label:'帧数',visibleWhen:gif}),
  frame_duration_ms:number(20,undefined,200,{label:'每帧时长（毫秒）',step:10,visibleWhen:gif}),
}, images(16))

const seedream = {size: pixels, resolution:res('1K 2K 4K','2K'), watermark:flag(false),
  sequential_image_generation: options('disabled auto','disabled',{optionLabels:{disabled:'关闭',auto:'自动组图'}}),
  sequential_image_generation_options: countGroup(15),
}
addImage( 'seedream-4-0 seedream-4-5', {...seedream, n:n(15),
  optimize_prompt_options: object({mode: options('standard fast','standard',{label:'速度'})},{label:'提示词优化'}),
}, images(10))
addImage( 'seedream-5-0-lite', {...seedream, resolution: res('2K 3K','2K'), output_format: format('jpeg png','jpeg'), n:n(15)}, images(10))
const layerFields = {size:pixels, resolution:res('1K 1.5K 2K','1K'), watermark:flag(false),
  background:options('opaque transparent','opaque'), output_format:format('jpeg png','jpeg'),
  layer_decomposition:flag(false,{label:'图层拆分'}),
  optimize_prompt_options:object({mode: options('standard fast','standard',{label:'速度'})},{label:'提示词优化'}),
}
addImage('seedream-5-0-pro seedream-5-0-flash',layerFields,images(10),{fixed:{n:1},usage:'可进行多图融合。图层拆分需要参考图。'})
addImage('seedream-5-5',{
  ...layerFields, output_format:format('png jpeg','png'), text_recognition:flag(false,{label:'文字识别'}),
  layer_decomposition_options:object({max_images:number(1,16,undefined,{label:'最大图层数'})},{label:'图层选项',visibleWhen:{layer_decomposition:true}}),
}, images(10))

addImage('kling-v4-image',{
  size:ratio('auto ' + ratios7 + ' 21:9','auto'), resolution:res('2K 4K 8K','2K'),
  n:n(9), sequential_image_generation:options('disabled auto','disabled'), sequential_image_generation_options:countGroup(4),
}, images(10,false))
const longImageMedia = (min,max) => ({intent:min?'reference':'text',images:{min,max},videos:{min:0,max:0},audios:{min:0,max:0}})
const ecommerce = {skill:'ecommerce'}
const webpage = {skill:'webpage'}
add('image','kling-v4-long-image',{
  skill:options('ecommerce webpage','ecommerce',{label:'长图类型',optionLabels:{ecommerce:'电商详情',webpage:'网页设计'},
    inputModes:{ecommerce:longImageMedia(1,3),webpage:longImageMedia(0,2)}}),
  length:options('short medium long','medium',{label:'长度',visibleWhen:{skill:'ecommerce'},optionLabels:{short:'短',medium:'中',long:'长'}}),
  resolution:res('1K 2K'), watermark:flag(false),
}, [
  ref('image_urls','image',3,{array:true,visibleWhen:ecommerce}),
  ref('image_with_roles','image',1,{role:'style_image',array:true,itemRole:'style',visibleWhen:webpage}),
  ref('image_with_roles','image',1,{role:'subject_image',array:true,itemRole:'subject',visibleWhen:webpage}),
], {fixed:{n:1},referenceSyntax:'en-at-numbered'})

addImage('flux-kontext-pro flux-kontext-max',{
  size:ratio(), output_format:format('png jpeg'), seed, safety_tolerance:number(0,6,2,{label:'安全容差'}),
  prompt_upsampling:flag(false,{label:'扩写提示词'}),
}, images(1),{fixed:{n:1}})
const flux2 = {
  size:ratio(), resolution:options('1MP 2MP 4MP','2MP'),
  width:number(256,2048), height:number(256,2048), output_format:format('jpeg png','jpeg'),
  seed, prompt_upsampling:flag(false,{label:'扩写提示词'}), safety_tolerance:number(0,6,2,{label:'安全容差'}),
}
addImage('flux-2-pro flux-2-max',flux2,images(8),{fixed:{n:1}})
addImage('flux-2-flex',{...flux2,steps:number(1,50,50,{label:'推理步数'}),guidance:decimal(1.5,10,5,{label:'提示词引导'})},images(8),{fixed:{n:1}})
const flux3 = {aspect_ratio:ratio('auto ' + ratios,'auto'),resolution:res('1k 1.5k 2k 4k','1k'),safety_tolerance:number(0,6,2,{label:'安全容差'})}
addImage('flux-3-image',{...flux3,grounding:flag(true)},images(10),{fixed:{n:1}})
addImage('flux-3-image-klein',{
  ...flux3,resolution:res('768sq 1k 1.5k 2k 4k','1k'),seed,steps:number(1,50,16,{label:'推理步数'}),output_format:format('jpeg png','jpeg'),
},images(10),{fixed:{n:1}})
addImage('hy-image-v3.5-preview',{size:pixels,resolution:res(),width:number(1,4096),height:number(1,4096),negative_prompt:line(4000,{control:'textarea'}),seed},images(5),{fixed:{n:1}})
addImage('muse-image-ext',{size:ratio('auto ' + ratios7,'auto'),language:options('zh en',undefined)},images(9,false),{fixed:{n:1}})

const mai = {size:pixels,resolution:res('1K 2K'),width:number(1,2048),height:number(1,2048)}
addImage('mai-image-2.6 mai-image-2.6-flash',{...mai,auto_aspect_ratio:flag(false),web_grounding:flag(false)},images(5),{fixed:{n:1}})
addImage('mai-image-2.7 mai-image-2.7-flash',{...mai,background:options('opaque transparent')},images(5),{fixed:{n:1}})
addImage('minimax-h3-image minimax-h3-image-highspeed',{
  size:pixels,resolution:res('1K 2K 4K','4K'),n:n(10),prompt_optimizer:flag(true),
  output_format:format('jpeg png webp','jpeg'),output_compression:number(0,100,95,{visibleWhenAny:[{output_format:'jpeg'},{output_format:'webp'}]}),watermark:flag(false),
},images(16))
const qwen = {size:ratio(),resolution:res('1K 2K'),n:n(6),negative_prompt:line(2000,{control:'textarea'})}
addImage('qwen-image-2.0 qwen-image-2.0-pro',qwen,images(3))
addImage('qwen-image-3.0 qwen-image-3.0-pro',{...qwen,prompt_extend:flag(false)},images(3))
addImage('z-image-turbo',{size:ratio(),resolution:res('1K 2K'),prompt_extend:flag(false)},[])
addImage('imagen-4.0-apimart',{size:ratio('1:1 3:4 4:3 9:16 16:9'),n:n(4)},[])
addImage('grok-imagine-1.0-apimart grok-imagine-1.5-apimart',{size:ratio(),n:n(4)},[])
addImage('grok-imagine-1.0-edit-apimart',{size:ratio()},[ref('image_urls','image',1,{array:true,inline:true,min:1})],{fixed:{n:1}})
addImage('grok-imagine-1.5-edit-apimart',{size:ratio()},[ref('image_urls','image',5,{array:true,min:1})],{
  upstream:'grok-imagine-1.5-apimart',fixed:{n:1},...generationMode('reference','参考图编辑'),
})
const officialGrokRatios = 'auto 1:1 16:9 9:16 4:3 3:4 3:2 2:3 2:1 1:2 19.5:9 9:19.5 20:9 9:20'
const officialGrok = {size:ratio(officialGrokRatios,'1:1'),resolution:res('1k 2k','1k'),n:n(10)}
addImage('grok-imagine-image',officialGrok,images(5))
addImage('grok-imagine-image-quality',officialGrok,images(3))
addImage('grok-imagine-2.0-ext',{size:ratio('auto ' + wideRatios,'auto'),resolution:res('1k 2k','1k'),n:n(12)},images(5),{usage:'文生图最多 12 张；使用参考图时仅支持生成 1 张。'})
addImage('grok-imagine-image-2.0',{aspect_ratio:ratio('auto ' + wideRatios,'auto'),resolution:res('1k 2k','1k'),quality:options('low medium','medium'),n:n(10)},images(3))
addImage('wan2.7-image wan2.7-image-pro',{
  size:options('1K 2K 4K','2K'),n:n(9),negative_prompt:line(2000,{control:'textarea'}),
  watermark:flag(false),seed,thinking_mode:flag(true,{label:'思考模式'}),enable_sequential:flag(false),
},images(10))
addImage('midjourney',{
  size:ratio(),speed:options('relax fast turbo','relax'),version:options('8.2 8.1 7 6.1 6 5.2 5.1','8.2'),
  quality:options('0.25 0.5 1 2'),seed,negative_prompt:line(1000,{control:'textarea'}),
  stylize:number(0,1000,undefined,{label:'风格化'}),chaos:number(0,100,undefined,{label:'变化幅度'}),
  weird:number(0,3000,undefined,{label:'怪异度'}),tile:flag(undefined,{label:'平铺'}),
  raw:flag(undefined,{label:'原始风格'}),draft:flag(undefined,{label:'草稿'}),hd:flag(undefined,{label:'高清'}),
},images(4),{...imageAndText('垫图生成'),endpoint:'/v1/midjourney/generations',action:'imagine',usage:'原生 Midjourney 提示词参数也可直接写在提示词末尾。'})
