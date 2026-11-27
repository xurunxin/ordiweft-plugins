// SPDX-License-Identifier: Apache-2.0
import { add, decimal, flag, generationMode, imageAndText, images, line, list, number, object, officialFallback, options, publicURL, ratios, ref, wideRatios } from './definitions.mjs'

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
  'gemini-3.1-flash-lite-image': 'Nano banana 2 Lite',
  'gemini-3.1-flash-lite-image-ext': 'Nano banana 2 Lite · Ext',
}
for (const [id, name] of Object.entries(nanoLabels)) {
  const v31 = id.startsWith('gemini-3.1')
  if (id.includes('lite')) {
    addImage(id,{size:ratio('auto '+ratios,'auto')},[ref('image_urls','image',14,{array:true,inline:true,maxBytes:10000000})],{
      name,fixed:{n:1,resolution:'1K'},usage:'固定 1K，支持最多 14 张参考图。',
    })
    continue
  }
  const official = id.endsWith('official')
  addImage( id, {
    size: ratio('auto ' + (v31 ? wideRatios : ratios), 'auto'),
    ...(v31 ? {resolution: res('0.5K 1K 2K 4K')} : id.startsWith('gemini-3-pro') ? {resolution: res()} : {}),
    ...(!official ? {official_fallback:officialFallback} : {}),
    ...(official ? {n: n(4)} : {}),
    ...(v31 ? {google_search: flag(false), google_image_search: flag(false)} : {}),
  }, images(id.startsWith('gemini-2.5') ? 3 : 14), {name, fixed: !official ? {n: 1} : undefined})
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
addImage('gpt-image-2', {size:ratio(gptRatios), resolution:res('1k 2k 4k','1k'), n:n(1), official_fallback:officialFallback}, images(15))
addImage('gpt-image-2-official', {...commonGPT, size:ratio(gptRatios), resolution:res('1k 2k 4k','1k')}, masked(16))
const nextGPT = {...commonGPT, quality: options('auto low medium high xhigh max','auto'), resolution: res('1k 2k 4k','1k'),
  size: pixels, background: options('auto opaque transparent','auto')}
addImage( 'gpt-image-2.5-flare gpt-image-2.5-sunburst', nextGPT, masked(16))
addImage( 'gpt-image-2.5-ext', {version: options('flare sunburst','flare'), size: ratio('auto ' + ratios, 'auto'), resolution: res(), n: n(4)}, images(16))
const gif = {output_format:'gif'}
const svg = {output_format:'svg'}
const svgFields = {
  model:line(128,{label:'SVG 编写模型',required:true}),
  reasoning:object({effort:options('none minimal low medium high xhigh max',undefined,{label:'推理强度'})},{label:'SVG 推理'}),
}
const complexity = options('low medium high',undefined,{label:'规划强度'})
const markup = ref('markup_url','image',1,{role:'markup',inline:true})
addImage( 'gpt-image-2.6-flare gpt-image-2.6-sunburst', {
  ...nextGPT, complexity, svg:object(svgFields,{label:'SVG 配置',visibleWhen:svg,required:true}),
  safety_identifier:line(128,{label:'终端用户标识',description:'填入稳定的匿名标识，例如用户名的哈希。'}),
  output_format: format('png jpeg webp gif svg'),
  frame_count: number(2, 16, 9, {label:'帧数', visibleWhen:gif}),
  frame_duration_ms: number(20, undefined, 200, {label:'每帧时长（毫秒）', step:10, visibleWhen:gif}),
}, [...masked(16),markup], {usage: '静态图支持参考图、蒙版或标注层。GIF 使用单帧像素尺寸。官方 SVG 另按编写模型的 Token 结算。'})
addImage( 'gpt-image-2.6-ext', {
  version:options('flare sunburst','flare'), size:ratio('auto '+ratios,'auto'), resolution: res('1k 2k 4k','1k'), n:n(1),
  quality:nextGPT.quality, complexity, svg:object(svgFields,{label:'SVG 配置',visibleWhen:svg}),
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
  ...layerFields, resolution:res('1K 1.5K 2K','2K'), background:options('auto opaque transparent','auto',{visibleWhen:{layer_decomposition:false}}),
  output_format:format('png jpeg','png'),
  optimize_prompt_options:object({mode:options('standard','standard',{label:'标准'})},{label:'提示词优化'}),
  text_recognition:flag(false,{deprecated:true}),
  layer_decomposition_options:object({max_images:number(2,100,30,{label:'最大输出数（含底图）',required:true}),text_recognition:flag(false,{label:'富文本 OCR',description:'可尝试留空拆分提示词，以便返回识别文字。'})},{label:'图层选项',default:{max_images:30},visibleWhen:{layer_decomposition:true}}),
}, images(10),{fixed:{n:1},optionalPrompt:true,
  outputCount:[{when:{layer_decomposition:true},parameter:'layer_decomposition_options.max_images',maximum:100}],
  usage:'图层拆分接入一张图片，省略提示词时自动拆分。'})

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
addImage('qwen-image-3.0 qwen-image-3.0-pro',{...qwen,prompt_extend:flag(false),prompt_extend_mode:options('direct agent',undefined,{label:'提示词改写方式',visibleWhen:{prompt_extend:true},optionLabels:{direct:'直接改写',agent:'智能改写（文生图）'}})},images(3))
addImage('z-image-turbo',{size:ratio(),resolution:res('1K 2K'),prompt_extend:flag(false)},[])
addImage('imagen-4.0-apimart',{size:ratio('1:1 3:4 4:3 9:16 16:9'),n:n(4)},[])
addImage('grok-imagine-1.0-apimart grok-imagine-1.5-apimart',{size:ratio(),n:n(4)},[])
addImage('grok-imagine-1.0-edit-apimart',{size:ratio()},[ref('image_urls','image',1,{array:true,inline:true,min:1})],{fixed:{n:1}})
addImage('grok-imagine-1.5-edit-apimart',{size:ratio()},[ref('image_urls','image',5,{array:true,min:1})],{
  upstream:'grok-imagine-1.5-apimart',fixed:{n:1},...generationMode('reference','参考图编辑'),
})
const officialGrokRatios = 'auto 1:1 16:9 9:16 4:3 3:4 3:2 2:3 2:1 1:2 19.5:9 9:19.5 20:9 9:20'
const officialGrok = {size:ratio(officialGrokRatios,'auto'),resolution:res('1k 2k','1k'),n:n(10)}
addImage('grok-imagine-image',officialGrok,images(5),{maxPrompt:8000})
addImage('grok-imagine-image-quality',officialGrok,images(3),{maxPrompt:8000})
const regionCoordinate = label => decimal(0,1,undefined,{label,required:true})
const regionPoint = object({x:regionCoordinate('横坐标'),y:regionCoordinate('纵坐标')})
const regionRing = label => list(regionPoint,{label,required:true,description:'按轮廓顺序填写至少 3 个点。坐标范围为 0–1。'})
addImage('grok-imagine-2.0-ext-region-edit',{
  selection_mode:options('rectangle polygon objects','rectangle',{label:'选区方式',section:'common',optionLabels:{rectangle:'矩形',polygon:'多边形',objects:'对象索引'}}),
  polygons:list(object({outer:regionRing('外轮廓'),holes:list(object({points:regionRing('孔洞轮廓')}),{label:'内部孔洞'})}),
    {label:'多边形选区',section:'common',required:true,visibleWhen:{selection_mode:'polygon'}}),
  object_indices:list(number(0,1000000),{label:'对象索引',section:'common',required:true,visibleWhen:{selection_mode:'objects'},
    description:'使用 APIMart 分段结果中的原始对象索引。提交前核对索引是否属于当前原图。'}),
  regions:list(object({x1:regionCoordinate('左'),y1:regionCoordinate('上'),x2:regionCoordinate('右'),y2:regionCoordinate('下')}),
    {required:true,label:'矩形选区',section:'common',visibleWhen:{selection_mode:'rectangle'},description:'坐标为相对整图的 0–1 小数，左上为 (0,0)，右下为 (1,1)。最多 16 个选区。'}),
},[],{upstream:'grok-imagine-2.0-ext',name:'Grok Imagine 2.0 · 局部编辑',hidden:true,
  taskContinuation:{sourceModels:['grok-imagine-2.0-ext'],sourceWhen:{n:1},actionLabel:'局部编辑'},
  usage:'沿用已完成的单图任务。设置选区和修改提示词后确认生成。'})
addImage('grok-imagine-2.0-ext',{
  size:ratio('auto 1:1 2:3 3:2 3:4 4:3 9:16 16:9','auto'),
  resolution:{...res('1k 2k','1k'),deprecated:true},n:n(12),
},images(5,false),{usage:'文生图最多 12 张。使用参考图时生成一张。画面比例为自动时由接口决定。'})
const officialGrok2 = {aspect_ratio:ratio(officialGrokRatios,'auto'),resolution:res('1k 2k','1k'),n:n(10)}
addImage('grok-imagine-image-2.0',{...officialGrok2,quality:options('low medium','medium')},[ref('image_urls','image',3,{array:true,maxBytes:10*1024*1024})],{
  ...generationMode('text','文生图'),maxPrompt:8000,
})
addImage('grok-imagine-image-2.0-reference',officialGrok2,[ref('image_urls','image',3,{array:true,min:1,maxBytes:10*1024*1024})],{
  upstream:'grok-imagine-image-2.0',name:'Grok Imagine Image 2.0 · 参考图编辑',...generationMode('reference','参考图编辑'),maxPrompt:8000,
})
const rectangle = object(Object.fromEntries(['x1','y1','x2','y2'].map(key => [key,number(0,undefined,undefined,{label:key,required:true})])),{label:'框选区域'})
for (const id of ['wan2.7-image','wan2.7-image-pro']) {
  addImage(id,{
    size:line(24,{label:'尺寸或比例',description:'可填写像素尺寸、宽高比或分辨率档位。'}),
    resolution:res(id.endsWith('-pro')?'1K 2K 4K':'1K 2K','2K'), n:n(12), negative_prompt:line(2000,{control:'textarea'}),
    watermark:flag(false), seed:number(0,2147483647), thinking_mode:flag(undefined,{label:'思考模式',visibleWhen:{enable_sequential:false}}), enable_sequential:flag(false),
    bbox_list:list(object({boxes:list(rectangle,{label:'像素坐标'})},{label:'参考图'}),{label:'框选编辑区域',description:'按参考图顺序各添加一项，每张图最多 2 个框。无需框选的图片保留空项。'}),
    color_palette:list(object({hex:line(7,{label:'十六进制色值',required:true}),ratio:line(7,{label:'占比（如 10.5%）',required:true})}),{label:'配色表',description:'填写 3 至 10 个颜色，占比合计 100%。',visibleWhen:{enable_sequential:false}}),
  },images(10))
}
addImage('midjourney-follow-up',{
  operation:options('upscale variation reroll','upscale',{label:'续作方式',section:'common',optionLabels:{upscale:'从四宫格选图',variation:'生成变体',reroll:'重生成四宫格'}}),
  upscale_style:options('normal subtle creative legacy_2x legacy_4x','normal',{label:'放大方式',section:'common',
    visibleWhen:{operation:'upscale'},optionLabels:{normal:'从四宫格取图',subtle:'高清放大 · 细节保持',creative:'高清放大 · 创意增强',legacy_2x:'V5 高清 2×',legacy_4x:'V5 高清 4×'}}),
  speed:options('relax fast turbo','fast',{label:'生成速度'}),
  index:number(1,4,1,{label:'四宫格位置',visibleWhenAny:[{operation:'upscale'},{operation:'variation'}]}),
},[],{upstream:'midjourney',name:'Midjourney 原图续作',hidden:true,promptKey:false,prompt:{required:false,label:'原图'},
  taskContinuation:{sourceModels:['midjourney'],actionLabel:'Midjourney 原图续作'},endpoint:'/v1/midjourney/generations/upscale',
  usage:'从所选四宫格取图、高清放大、生成变体或重生成。高清档位取决于原任务提供的按钮。'})

addImage('midjourney-inpaint',{speed:options('relax fast turbo','fast',{label:'生成速度',section:'common'})},
  [ref('mask_url','image',1,{role:'mask',min:1,inline:true,maxBytes:12*1024*1024})],{
  upstream:'midjourney',name:'Midjourney 遮罩重绘',hidden:true,optionalPrompt:true,maxPrompt:4000,
  taskContinuation:{sourceModels:['midjourney-follow-up'],sourceWhen:{operation:'upscale'},actionLabel:'遮罩重绘',mask:true,allowOptionalPrompt:true},
  endpoint:'/v1/midjourney/generations/modal',usage:'在固定的放大单图上画好遮罩。提示词留空时沿用原任务描述。',
})

addImage('midjourney',{
  size:ratio(),speed:options('relax fast turbo','relax'),version:options('8.2 8.1 7 6.1 6 5.2 5.1','8.2'),
  quality:options('0.25 0.5 1 2'),seed,negative_prompt:line(1000,{control:'textarea'}),
  stylize:number(0,1000,undefined,{label:'风格化'}),chaos:number(0,100,undefined,{label:'变化幅度'}),
  weird:number(0,3000,undefined,{label:'怪异度'}),tile:flag(undefined,{label:'平铺'}),
  style:line(64,{label:'风格预设'}),niji:flag(undefined,{label:'Niji 模式'}),
  iw:decimal(0,3,undefined,{label:'垫图权重'}),cw:number(0,100,undefined,{label:'角色权重'}),
  sw:number(0,1000,undefined,{label:'风格权重'}),dw:decimal(0,100,undefined,{label:'深度权重'}),
  cref:publicURL('角色参考图网址'),sref:publicURL('风格参考图网址'),dref:publicURL('深度参考图网址'),
  repeat:number(1,40,1,{label:'重复生成',description:'1 为单次，2 至 40 为重复生成。'}),stop:number(10,100,undefined,{label:'提前停止（%）'}),
  extra:line(1000,{label:'附加参数'}),
  raw:flag(undefined,{label:'原始风格'}),draft:flag(undefined,{label:'草稿'}),hd:flag(undefined,{label:'高清'}),
},images(4),{...imageAndText('垫图生成'),endpoint:'/v1/midjourney/generations',action:'imagine',outputCount:[{parameter:'repeat',maximum:40}],usage:'原生 Midjourney 提示词参数也可直接写在提示词末尾。'})
