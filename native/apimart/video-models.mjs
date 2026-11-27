// SPDX-License-Identifier: Apache-2.0
import { add, flag, frames, images, line, list, number, object, options, ref, textAndReference, videoMode, videoRatios } from './definitions.mjs'

const landscape = '16:9 9:16'
const smallRatios = '16:9 9:16 1:1'
const res = (choices, value = '720p') => options(choices, value)
const ratio = (choices = videoRatios, value = '16:9') => options(choices, value)
const seconds = (min, max, value = 5) => number(min, max, value)
const durationChoices = (list, value) => options(list, value)
const negative_prompt = line(2500, {control:'textarea'})
const seed = number(-1, 4294967295)
const watermark = flag(false)
const prompt_optimizer = flag(true)
const img = (n, inline = false, min = 0) => [ref('image_urls','image',n,{array:true,inline,min})]
const video = (n, extra = {}) => ref('video_urls','video',n,{array:true,...extra})
const audio = (n, extra = {}) => ref('audio_urls','audio',n,{array:true,...extra})
const multimodal = (i,v,a, inline = false) => [...img(i,inline),...(v?[video(v)]:[]),...(a?[audio(a)]:[])]
const videoOnly = field => [ref(field,'video',1,{min:1,array:field.endsWith('_urls'),role:'source_video'})]
const taggedFrames = (last = true) => ['first_frame',...(last?['last_frame']:[])].map((role,index) =>
  ref('image_with_roles','image',1,{role,itemRole:role,array:true,min:index===0?1:0}))
const basic = (duration, resolution, aspect_ratio = ratio()) => ({duration,resolution,aspect_ratio})
const mention = (upstream, name, fixed, usage) => ({upstream,name,fixed,usage})
const imageVideo = (options = {}) => videoMode('first_last_frame','图生视频',options)
const editVideo = videoMode('edit','视频编辑',{manualOnly:true})
const extendVideo = videoMode('extend','视频延长',{manualOnly:true})

const zoe = {duration:seconds(4,30), resolution:res('480p 720p 1080p 2k 4k'), aspect_ratio:ratio('adaptive '+videoRatios,'adaptive'),
  generate_audio:flag(true), seed, return_last_frame:flag(false,{label:'返回尾帧'}), enhance_prompt:flag(undefined)}
add('video','apimart-zoe',zoe,multimodal(30,10,10),{...textAndReference(),audioRequiresVisual:true,usage:'可使用图像、视频或图像／视频与音频的组合。视频、音频均为参考素材。'})
add('video','apimart-zoe-frames',{...zoe,aspect_ratio:undefined},frames(true),{...mention('apimart-zoe','APIMart Zoe · 首尾帧',{},'首帧必填，尾帧可选。'),...imageVideo()})
add('video','apimart-zoe-edit',{resolution:zoe.resolution,generate_audio:zoe.generate_audio},videoOnly('video_urls'),{...mention('apimart-zoe','APIMart Zoe · 视频编辑',{omni_reference_task_type:'edit'}),...editVideo})

const v1 = {...basic(seconds(2,12),res('480p 720p 1080p','1080p')),seed}
add('video','seedance-1-0-pro-fast seedance-1-0-pro-quality',v1,img(2))
const seedance2 = {
  duration:seconds(4,15), resolution:res('480p 720p 1080p'), size:ratio('adaptive '+videoRatios),
  generate_audio:flag(true), seed, return_last_frame:flag(false,{label:'返回尾帧'}),
}
const seedance2Prompt = {maxPrompt:4000,referenceSyntax:'cn-numbered'}
const seedance2Reference = {audioRequiresVisual:true,usage:'参考音频需搭配图片或视频。'}
add('video','seedance-2.0 seedance-2.0-fast',seedance2,multimodal(9,3,3),{...seedance2Prompt,...seedance2Reference,...textAndReference()})
add('video','seedance-2.0-mini',{...seedance2,resolution:res('480p 720p')},multimodal(9,3,3),{
  ...seedance2Prompt,...seedance2Reference,...textAndReference(),maxPrompt:undefined,
})
add('video','seedance-2.0-face seedance-2.0-fast-face',seedance2,multimodal(9,3,3),{...seedance2Prompt,...seedance2Reference})
for (const id of ['seedance-2.0','seedance-2.0-fast','seedance-2.0-mini']) {
  const mini = id.endsWith('mini')
  add('video',id+'-frames',{...seedance2,resolution:mini?res('480p 720p'):seedance2.resolution,size:undefined},taggedFrames(),{
    ...mention(id,id+' · 首尾帧',{},'首帧必填，尾帧可选。'),...imageVideo(),...seedance2Prompt,maxPrompt:mini?undefined:seedance2Prompt.maxPrompt,
  })
}
const seedance25 = {...seedance2,duration:seconds(4,30),resolution:res('480p 720p 1080p 4k'),
  size:ratio('adaptive '+videoRatios,'adaptive'),watermark,web_search:flag(false,{label:'联网搜索'})}
const seedance25Prompt = {referenceSyntax:'cn-at-numbered'}
for (const id of ['seedance-2.5','seedance-2-5-flash']) {
  const flash = id.endsWith('flash')
  const fields = {...seedance25,resolution:flash?res('480p 720p'):seedance25.resolution}
  add('video',id,fields,multimodal(flash?10:30,flash?5:10,flash?5:10),{...seedance25Prompt,...textAndReference()})
  add('video',id+'-frames',{...fields,size:undefined},taggedFrames(),{...mention(id,id+' · 首尾帧',{size:'adaptive'},'首帧必填，尾帧可选。'),...imageVideo(),...seedance25Prompt})
  add('video',id+'-edit',{resolution:fields.resolution,generate_audio:fields.generate_audio,watermark},videoOnly('video_urls'),{...mention(id,id+' · 视频编辑',{omni_reference_task_type:'edit',size:'adaptive'},'时长和画幅跟随源视频。'),...editVideo,...seedance25Prompt})
  add('video',id+'-extend',{duration:fields.duration,resolution:fields.resolution,generate_audio:fields.generate_audio,watermark},videoOnly('video_urls'),{...mention(id,id+' · 视频续写',{omni_reference_task_type:'extend',size:'adaptive'}),...extendVideo,...seedance25Prompt})
}
const seedanceDraftFields = {...seedance25,resolution:undefined}
add('video','seedance-2.5-draft',seedanceDraftFields,multimodal(30,10,10),{
  ...seedance25Prompt,upstream:'seedance-2.5',name:'Seedance 2.5 · 480p 草稿',fixed:{draft:true,resolution:'480p'},
  usage:'先生成 480p 草稿，满意后可在该版本创建 1080p 正式片。草稿从上游创建起保留 3 天的续作资格。',
})
add('video','seedance-2.5-draft-frames',{...seedanceDraftFields,size:undefined},taggedFrames(),{
  ...seedance25Prompt,upstream:'seedance-2.5',name:'Seedance 2.5 · 480p 首尾帧草稿',fixed:{draft:true,resolution:'480p',size:'adaptive'},
  usage:'首帧必填，尾帧可选。生成后可从该版本创建 1080p 正式片。',
})
add('video','seedance-2.5-final-1080p',{watermark},[],{
  upstream:'seedance-2.5',name:'Seedance 2.5 · 1080p 正式片',hidden:true,
  promptKey:false,prompt:{label:'原草稿',required:false},fixed:{resolution:'1080p'},
  usage:'沿用所选 480p 草稿的内容、时长、画幅和参考，生成 1080p 正式片。',
  taskContinuation:{sourceModels:['seedance-2.5-draft','seedance-2.5-draft-frames'],actionLabel:'生成 1080p 正式片',maxAgeSeconds:259200},
})

for (const id of ['veo3.1-fast','veo3.1-lite','veo3.1-quality']) {
  add('video',id,{...basic(undefined,res(id.endsWith('lite')?'720p 1080p':'720p 1080p 4k'),ratio(landscape)),
    generation_type:options('text-to-video image-to-video reference-to-video',undefined,{label:'生成方式'}),
  },img(3),{fixed:{duration:8}})
}
add('video','veo3.1-fast-official veo3.1-quality-official',{
  ...basic(durationChoices([4,6,8],8),res('720p 1080p 4k'),ratio(landscape)),negative_prompt,seed,
  generate_audio:flag(false),enhance_prompt:flag(true),resize_mode:options('pad crop','pad',{label:'素材适配'}),
},frames())

add('video','MiniMax-Hailuo-02',{duration:durationChoices([6,10],6),resolution:res('512p 768p 1080p','768p'),
  prompt_optimizer,fast_pretreatment:flag(false,{label:'快速预处理'}),watermark},frames())
add('video','MiniMax-Hailuo-2.3 MiniMax-Hailuo-2.3-Fast',{duration:durationChoices([6,10],6),resolution:res('768p 1080p','768p'),
  prompt_optimizer,fast_pretreatment:flag(false,{label:'快速预处理'}),watermark},frames(false,''))
const h3 = {...basic(seconds(4,15),res('768P 2K','2K'),ratio('adaptive '+videoRatios)),watermark}
const h3Max = {...h3,duration:seconds(5,15),resolution:res('480P 768P 1080P','768P'),
  watermark:flag(false,{visibleWhenAny:[{resolution:'480P'},{resolution:'768P'}]})}
const h3Prompt = {maxPrompt:7000,referenceSyntax:'cn-numbered'}
const h3Frames = {...h3Prompt,...imageVideo({minInputs:1}),usage:'可接入首帧、尾帧或两者。画幅跟随输入图片。'}
const h3Reference = {...h3Prompt,...textAndReference(),audioRequiresVisual:true,usage:'最多 9 张参考图、3 段参考视频和 3 段参考音频。音频需搭配图片或视频。'}
add('video','MiniMax-H3',h3,multimodal(9,3,3),h3Reference)
add('video','MiniMax-H3-Max',h3Max,multimodal(9,3,3),h3Reference)
add('video','MiniMax-H3-frames',h3,frames(),{upstream:'MiniMax-H3',name:'MiniMax H3 · 首尾帧',...h3Frames})
add('video','MiniMax-H3-Max-frames',h3Max,frames(),{upstream:'MiniMax-H3-Max',name:'MiniMax H3 Max · 首尾帧',...h3Frames})
add('video','MiniMax-H3-Regeneration',{},[],{
  name:'MiniMax H3 · 2K 再生成',hidden:true,promptKey:false,prompt:{label:'原视频',required:false},
  usage:'沿用所选 MiniMax H3 768P 成片与原参考，生成 2K 视频。',
  taskContinuation:{sourceModels:['MiniMax-H3','MiniMax-H3-frames'],sourceWhen:{resolution:'768P'},actionLabel:'升为 2K'},
})

const flux = {...basic(seconds(5,20),res('hd fhd','hd'),ratio('auto 2:1 '+videoRatios,'auto')),
  audio:flag(true),safety_tolerance:number(0,6,2,{label:'安全容差'})}
add('video','flux-3-video',flux,img(10,true),{
  ...imageVideo({textLabel:'文生视频'}),usage:'按顺序连接 1 至 10 张关键帧。',
})
add('video','flux-3-video-edit',{resolution:flux.resolution},videoOnly('video_url'),{...mention('flux-3-video','FLUX 3 Video · 视频续写',{}),...videoMode('extend','视频续写')})
add('video','skyreels-v4-fast skyreels-v4-std',{...basic(seconds(3,15),res('720p 1080p','1080p')),prompt_optimizer},[
  ...frames(false,'end_frame_image'),ref('audio_url','audio',1),
])
add('video','skyreels-v4-std-reference',{...basic(seconds(3,15),res('720p 1080p','1080p')),prompt_optimizer},img(6),mention('skyreels-v4-std','SkyReels V4 · 参考图',{}))

const horse = {duration:seconds(3,15),resolution:res('480P 720P 1080P','1080P'),size:ratio(),seed,watermark}
for (const id of ['happyhorse-1.0','happyhorse-1.1']) {
  add('video',id,horse,img(9,true),id==='happyhorse-1.0'?videoMode('reference','参考图生视频',{textLabel:'文生视频'}):{})
  add('video',id+'-frames',horse,frames(true,''),{
    ...mention(id,id+' · 首帧',{}),...(id==='happyhorse-1.0'?videoMode('first_frame','图生视频'):{}),
  })
}
add('video','happyhorse-1.0-edit',{resolution:horse.resolution,audio_setting:options('auto keep mute','auto',{label:'音轨处理'})},[
  ...videoOnly('video_url'),...img(5,true),
],{...mention('happyhorse-1.0','HappyHorse 1.0 · 视频编辑',{}),...videoMode('edit','视频编辑')})

const wan = {duration:seconds(2,15),resolution:res('480P 720P 1080P','1080P'),size:ratio(),negative_prompt,prompt_extend:flag(true),seed,watermark}
const wan3Prompt = {maxPrompt:20000,referenceSyntax:'cn-short-numbered'}
for (const id of ['wan3.0-video','wan3.0-video-prime']) {
  add('video',id,{...wan,duration:seconds(2,30),size:ratio('adaptive '+videoRatios,'adaptive'),audio:flag(true)},multimodal(9,5,5),{...wan3Prompt,...textAndReference()})
  add('video',id+'-frames',{...wan,duration:seconds(2,30),size:undefined,audio:flag(true)},taggedFrames(),{...mention(id,id+' · 首尾帧',{size:'adaptive'},'首帧必填，尾帧可选。'),...imageVideo(),...wan3Prompt})
  add('video',id+'-edit',{resolution:wan.resolution},videoOnly('video_urls'),{...mention(id,id+' · 视频编辑',{generation_type:'edit'}),hidden:true})
}
add('video','wan2.7',wan,img(2),imageVideo({textLabel:'文生视频'}))
add('video','wan2.7-extend',{...wan,size:undefined},videoOnly('video_urls'),mention('wan2.7','Wan 2.7 · 视频续写',{}))
add('video','wan2.7-r2v',wan,[
  ref('image_with_roles','image',5,{itemRole:'reference_image',array:true}),video(5),
],{usage:'至少添加一份图片或视频参考，合计最多 5 份。'})
add('video','wan2.7-videoedit',{...wan,duration:durationChoices([0,2,3,4,5,6,7,8,9,10],0),audio_setting:options('auto keep mute','auto',{label:'音轨处理'})},[
  ...img(4),...videoOnly('video_urls'),
])
const wan25 = {duration:durationChoices([5,10],5),resolution:res('480p 720p 1080p'),negative_prompt,prompt_extend:flag(true),seed,watermark,audio:flag(true)}
add('video','wan2.5-preview',{...wan25,size:ratio()},[...img(1,true),ref('audio_url','audio',1)])
const wan26 = {...wan25,duration:durationChoices([5,10,15],5),resolution:res('720p 1080p'),
  shot_type:options('single multi','single',{label:'分镜方式'})}
add('video','wan2.6',{...wan26,aspect_ratio:ratio()},[...img(1),ref('audio_url','audio',1)])
add('video','wan2.6-i2v',wan26,[...img(1,false,1),ref('audio_url','audio',1)])
add('video','wan2.6-i2v-flash',{...wan26,duration:seconds(2,15),resolution:res('480p 720p 1080p','1080p')},[...img(1,true,1),ref('audio_url','audio',1)])

const kling = {...basic(seconds(3,15),undefined,ratio(smallRatios)),mode:options('std pro 4k','std'),negative_prompt,audio:flag(false),watermark}
const shots = {
  multi_shot:flag(false,{label:'多镜头'}),
  multi_prompt:list(object({prompt:line(2000,{label:'镜头提示词',required:true}),duration:line(3,{label:'时长（秒）',required:true})}),{label:'分镜列表',visibleWhen:{multi_shot:true}}),
}
add('video','kling-v2-6',{...kling,mode:options('std pro','std'),duration:durationChoices([5,10],5)},img(2))
add('video','kling-v3',{...kling,...shots},img(2),{optionalPrompt:true})
add('video','kling-v3-omni',{...kling,...shots},img(7),{optionalPrompt:true})
add('video','kling-video-o1',{...kling,mode:options('std pro','std'),duration:durationChoices([5,10],5),negative_prompt:undefined,audio:undefined,watermark:undefined},img(7))
add('video','kling-3.0-turbo',{...basic(seconds(3,15),res('720p 1080p'),ratio(smallRatios)),watermark},frames(false,''))
add('video','kling-v2-6-motion-control kling-v3-motion-control',{
  mode:options('std pro','std'),character_orientation:options('image video','image',{label:'人物朝向'}),
  keep_original_sound:options('yes no','yes',{label:'保留原声'}),
},[ref('image_url','image',1,{min:1,role:'first_frame'}),...videoOnly('video_url')],{optionalPrompt:true})
for (const id of ['kling-v4','kling-v4-flash']) {
  const flash = id.endsWith('flash')
  const fields = basic(seconds(3,flash?20:30),flash?undefined:res('720p 1080p'),ratio(smallRatios+' 21:9'))
  add('video',id,fields,[],{fixed:flash?{resolution:'720p'}:undefined})
  const fixed = flash ? {resolution:'720p'} : {}
  add('video',id+'-frames',{...fields,aspect_ratio:undefined},frames(true,flash?'':'last_frame_image'),{...mention(id,id+(flash?' · 首帧':' · 首尾帧'),fixed),...videoMode(flash?'first_frame':'first_last_frame','图生视频'),
    usage:flash?'仅接入一张首帧。':'接入首帧，可加尾帧。'})
  add('video',id+'-reference',{...fields,aspect_ratio:ratio('auto '+smallRatios+' 21:9','auto')},multimodal(10,5,flash?0:3),{
    ...mention(id,id+' · 多模态参考',{...fixed,omni_reference_task_type:'reference'}),
    ...videoMode('reference',flash?'参考生视频':'多模态参考生视频',{minInputs:1}),referenceSyntax:'en-at-numbered',
  })
  for (const [mode,label] of [['edit','编辑'],['extend','续写']]) {
    add('video',id+'-'+mode,{...fields,duration:mode==='edit'?number(3,flash?20:30):fields.duration},
      [...videoOnly('video_url'),...(mode==='edit'?img(10):[])],{
        ...mention(id,id+' · 视频'+label,{...fixed,omni_reference_task_type:mode}),
        ...(mode==='edit'?editVideo:videoMode('extend','视频续写',{manualOnly:true})),referenceSyntax:'en-at-numbered',
      })
  }
}

const vidu = {...basic(seconds(1,16),res('540p 720p 1080p')),seed,audio:flag(true)}
add('video','viduq3-pro viduq3-turbo',vidu,img(2))
add('video','viduq3',{...vidu,duration:seconds(3,16),audio:undefined},img(7,false,1))
add('video','viduq3-mix',{...vidu,resolution:res('720p 1080p'),audio:undefined},img(7,false,1))
const q4 = {...vidu,duration:seconds(3,16),resolution:res('540p 720p 1080p 2K 4K')}
add('video','viduq4-preview',q4,[],{hidden:true,name:'Vidu Q4 Preview',usage:'请选择图生视频或参考生视频。'})
add('video','viduq4-preview-frames',{...q4,aspect_ratio:undefined},frames(true,''),{
  ...mention('viduq4-preview','Vidu Q4 · 首帧',{},'仅接入一张首帧。'),...videoMode('first_frame','图生视频'),optionalPrompt:true,
})
add('video','viduq4-preview-reference',q4,[
  ref('image_with_roles','image',15,{itemRole:'reference_image',array:true,min:1,inline:true}),audio(3),
],{...mention('viduq4-preview','Vidu Q4 · 素材参考',{},'至少一张参考图，可搭配参考音频。'),
  ...videoMode('reference','参考生视频')})

for (const id of ['grok-imagine-1.0-video-apimart','grok-imagine-1.5-video-apimart']) {
  const fields = {duration:seconds(1,15,6),resolution:res('480p 720p 1080p','480p'),size:ratio(smallRatios)}
  add('video',id,fields,[],{usage:'参考和首尾帧模式可在模型列表中选择。'})
  add('video',id+'-reference',{...fields,resolution:res('480p 720p','480p')},img(14),mention(id,id+' · 参考图',{}))
  add('video',id+'-frames',fields,taggedFrames(),mention(id,id+' · 首尾帧',{}))
}
for (const id of ['grok-imagine-video','grok-imagine-video-1.5']) {
  const fields = basic(seconds(1,15,8),res(id.endsWith('1.5')?'480p 720p 1080p':'480p 720p','480p'),ratio(videoRatios+' 2:3 3:2'))
  add('video',id,fields,img(id.endsWith('1.5')?7:1))
  add('video',id+'-edit',{resolution:fields.resolution},videoOnly('video.url'),mention(id,id+' · 视频编辑',{}))
}

const pix = {duration:seconds(1,15),resolution:res('360p 540p 720p 1080p','540p'),audio:flag(false),seed:number(0,4294967295)}
add('video','pixverse-v6',{...pix,size:ratio(),negative_prompt,watermark},frames())
add('video','pixverse-v6-reference',{...pix,size:ratio(),negative_prompt,watermark},[ref('img_references','image',7,{array:true,min:1})],mention('pixverse-v6','PixVerse V6 · 多图参考',{}))
add('video','pixverse-v6-max',{...pix,aspect_ratio:ratio(videoRatios+' 2:3 3:2')},img(1))
add('video','muse-video-ext',{duration:seconds(2,10,10),aspect_ratio:options(videoRatios+' 2:3 3:2'),language:options('zh en')},img(9,true),{fixed:{resolution:'720p'}})

const omni = {duration:seconds(3,15,10),resolution:res('360p 720p 1080p 4k'),aspect_ratio:ratio(landscape)}
add('video','Omni-Flash-Ext',{...omni,duration:durationChoices([4,6,8,10],6)},[...img(3),video(1)],{usage:'零张或一张参考图，也可用三张图进行参考融合。'})
for (const id of ['gemini-omni-1.1-flash','gemini-omni-1.2-flash','gemini-omni-1.2-flash-ext']) {
  const fields = {...omni,duration:id.includes('1.1')?undefined:omni.duration}
  add('video',id,fields,img(10),id.endsWith('-ext')?videoMode('reference','多图参考生视频',{textLabel:'文生视频'}):{})
  add('video',id+'-frames',{...fields,duration:fields.duration?seconds(3,10,10):undefined},frames(true),mention(id,id+' · 首尾帧',{}))
  for (const [mode,label] of [['edit','编辑'],['extend','续写']]) {
    add('video',id+'-'+mode,{resolution:fields.resolution,duration:mode==='extend'&&fields.duration?seconds(3,10,10):undefined},
      videoOnly('video_urls'),mention(id,id+' · 视频'+label,{omni_reference_task_type:mode}))
  }
}
add('video','gemini-omni-flash-preview',{aspect_ratio:ratio(landscape)},[...img(16),video(1)],{fixed:{resolution:'720p'}})
