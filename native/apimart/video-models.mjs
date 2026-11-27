// SPDX-License-Identifier: Apache-2.0
import { add, flag, frames, images, line, list, number, object, officialFallback, options, publicURL, ref, textAndReference, generationMode, videoRatios } from './definitions.mjs'

const landscape = '16:9 9:16'
const smallRatios = '16:9 9:16 1:1'
const res = (choices, value = '720p') => options(choices, value)
const ratio = (choices = videoRatios, value = '16:9') => options(choices, value)
const seconds = (min, max, value = 5) => number(min, max, value)
const durationChoices = (list, value) => options(list, value)
const automaticSeconds = (min,max) => options([-1,...Array.from({length:max-min+1},(_,i)=>min+i)],5,{optionLabels:{'-1':'自动时长'}})
const videoFormat = options('mp4 mov','mp4')
const lastFrame = flag(false,{label:'返回尾帧'})
const withLastFrame = {outputCount:[{when:{return_last_frame:true},maximum:2}]}
const webSearch = flag(false,{label:'联网搜索'})
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
const imageVideo = (options = {}) => generationMode('first_last_frame','图生视频',options)
const textVideo = generationMode('text','文生视频')
const textAndFrames = (last = true, extra = {}) => generationMode(last?'first_last_frame':'first_frame','图生视频',{textLabel:'文生视频',...extra})
const textAndImageReference = (label = '参考图生视频', extra = {}) => generationMode('reference',label,{textLabel:'文生视频',...extra})
const continueTask = (sourceModels) => ({
  hidden:true, taskContinuation:{sourceModels,actionLabel:'视频续写'},
  usage:'填写续写内容，沿用已绑定的原视频。',
})
const editVideo = generationMode('edit','视频编辑',{manualOnly:true})
const extendVideo = generationMode('extend','视频延长',{manualOnly:true})

const zoe = {duration:automaticSeconds(4,30), resolution:res('480p 720p 1080p 2k 4k'), aspect_ratio:ratio('adaptive '+videoRatios,'adaptive'),
  generate_audio:flag(true), seed, return_last_frame:lastFrame, enhance_prompt:flag(undefined), output_format:videoFormat}
const zoePrompt = {...withLastFrame,optionalPrompt:true,maxPrompt:204800}
add('video','apimart-zoe',zoe,multimodal(30,10,10),{...zoePrompt,...textAndReference(),audioRequiresVisual:true,usage:'可使用图像、视频或图像／视频与音频的组合。视频、音频均为参考素材。'})
add('video','apimart-zoe-frames',{...zoe,aspect_ratio:undefined},frames(true),{...mention('apimart-zoe','APIMart Zoe · 首尾帧',{},'首帧必填，尾帧可选。'),...imageVideo(),...zoePrompt})
add('video','apimart-zoe-edit',{resolution:zoe.resolution,generate_audio:zoe.generate_audio,return_last_frame:lastFrame,output_format:videoFormat,enhance_prompt:zoe.enhance_prompt},videoOnly('video_urls'),{...mention('apimart-zoe','APIMart Zoe · 视频编辑',{omni_reference_task_type:'edit'}),...editVideo,...zoePrompt})
const zoeDraft = {...zoe,resolution:undefined}
const zoeDraftSettings = {...zoePrompt,upstream:'apimart-zoe',fixed:{draft:true,resolution:'480p'}}
add('video','apimart-zoe-draft',zoeDraft,multimodal(30,10,10),{
  ...zoeDraftSettings,...generationMode('reference','480p 样片',{manualOnly:true}),name:'APIMart Zoe · 480p 样片',audioRequiresVisual:true,
})
add('video','apimart-zoe-draft-frames',{...zoeDraft,aspect_ratio:undefined},frames(true),{
  ...zoeDraftSettings,...generationMode('first_last_frame','480p 首尾帧样片',{manualOnly:true}),name:'APIMart Zoe · 首尾帧样片',
})
add('video','apimart-zoe-final',{},[],{
  upstream:'apimart-zoe',name:'APIMart Zoe · 1080p 正片',hidden:true,promptKey:false,prompt:{label:'原样片',required:false},fixed:{resolution:'1080p'},
  taskContinuation:{sourceModels:['apimart-zoe-draft','apimart-zoe-draft-frames'],maxAgeSeconds:86400,actionLabel:'生成 1080p 正片'},
})

const v1 = {...basic(seconds(2,12),res('480p 720p 1080p','1080p')),seed}
for (const id of ['seedance-1-0-pro-fast','seedance-1-0-pro-quality']) {
  const last = id.endsWith('quality')
  add('video',id,v1,taggedFrames(last).map(ref=>({...ref,min:0})),textAndFrames(last))
}
const seedance2 = {
  duration:seconds(4,15), resolution:res('480p 720p 1080p'), size:ratio('adaptive '+videoRatios),
  generate_audio:flag(true), seed, return_last_frame:flag(false,{label:'返回尾帧'}),
}
const seedance2Prompt = {...withLastFrame,maxPrompt:4000,referenceSyntax:'cn-numbered'}
const seedance2Reference = {audioRequiresVisual:true,usage:'参考音频需搭配图片或视频。'}
add('video','seedance-2.0 seedance-2.0-fast',{...seedance2,web_search:webSearch},multimodal(9,3,3),{...seedance2Prompt,...seedance2Reference,...textAndReference()})
add('video','seedance-2.0-mini',{...seedance2,resolution:res('480p 720p'),web_search:webSearch},multimodal(9,3,3),{
  ...seedance2Prompt,...seedance2Reference,...textAndReference(),maxPrompt:undefined,
})
add('video','seedance-2.0-face seedance-2.0-fast-face',seedance2,multimodal(9,3,3),{...seedance2Prompt,...seedance2Reference})
for (const id of ['seedance-2.0','seedance-2.0-fast','seedance-2.0-mini']) {
  const mini = id.endsWith('mini')
  add('video',id+'-frames',{...seedance2,resolution:mini?res('480p 720p'):seedance2.resolution,size:undefined,web_search:webSearch},taggedFrames(),{
    ...mention(id,id+' · 首尾帧',{},'首帧必填，尾帧可选。'),...imageVideo(),...seedance2Prompt,maxPrompt:mini?undefined:seedance2Prompt.maxPrompt,
  })
}
const seedance25 = {...seedance2,duration:automaticSeconds(4,30),resolution:res('480p 720p 1080p 4k'),
  size:ratio('adaptive '+videoRatios,'adaptive'),watermark,web_search:webSearch,output_format:videoFormat}
const seedance25Prompt = {...withLastFrame,referenceSyntax:'cn-at-numbered'}
const seedance25Output = {watermark,return_last_frame:lastFrame,output_format:videoFormat}
for (const id of ['seedance-2.5','seedance-2-5-flash']) {
  const flash = id.endsWith('flash')
  const fields = {...seedance25,resolution:flash?res('480p 720p'):seedance25.resolution}
  add('video',id,fields,multimodal(flash?10:30,flash?5:10,flash?5:10),{...seedance25Prompt,...textAndReference()})
  add('video',id+'-frames',{...fields,size:undefined},taggedFrames(),{...mention(id,id+' · 首尾帧',{size:'adaptive'},'首帧必填，尾帧可选。'),...imageVideo(),...seedance25Prompt})
  add('video',id+'-edit',{resolution:fields.resolution,generate_audio:fields.generate_audio,...seedance25Output,web_search:webSearch},videoOnly('video_urls'),{...mention(id,id+' · 视频编辑',{omni_reference_task_type:'edit',size:'adaptive'},'时长和画幅跟随源视频。'),...editVideo,...seedance25Prompt})
  add('video',id+'-extend',{duration:fields.duration,resolution:fields.resolution,generate_audio:fields.generate_audio,...seedance25Output,web_search:webSearch},videoOnly('video_urls'),{...mention(id,id+' · 视频续写',{omni_reference_task_type:'extend',size:'adaptive'}),...extendVideo,...seedance25Prompt})
}
const seedanceDraftFields = {...seedance25,resolution:undefined}
add('video','seedance-2.5-draft',seedanceDraftFields,multimodal(30,10,10),{
  ...seedance25Prompt,upstream:'seedance-2.5',name:'Seedance 2.5 · 480p 草稿',fixed:{draft:true,resolution:'480p'},...generationMode('reference','480p 草稿',{manualOnly:true}),
  usage:'先生成 480p 草稿，满意后可在该版本创建 1080p 或 4K 正式片。草稿从上游创建起保留 3 天的续作资格。',
})
add('video','seedance-2.5-draft-frames',{...seedanceDraftFields,size:undefined},taggedFrames(),{
  ...seedance25Prompt,upstream:'seedance-2.5',name:'Seedance 2.5 · 480p 首尾帧草稿',fixed:{draft:true,resolution:'480p',size:'adaptive'},
  ...generationMode('first_last_frame','480p 首尾帧草稿',{manualOnly:true}),usage:'首帧必填，尾帧可选。生成后可从该版本创建 1080p 或 4K 正式片。',
})
add('video','seedance-2.5-final-1080p',{watermark,return_last_frame:flag(undefined,{label:'返回尾帧'}),output_format:options('mp4 mov'),resolution:res('1080p 4k','1080p')},[],{
  upstream:'seedance-2.5',name:'Seedance 2.5 · 草稿转正片',hidden:true,...withLastFrame,
  promptKey:false,prompt:{label:'原草稿',required:false},
  usage:'沿用样片内容。支持 1080p，原样片通道支持时可生成 4K。',
  taskContinuation:{sourceModels:['seedance-2.5-draft','seedance-2.5-draft-frames'],actionLabel:'草稿转正片',maxAgeSeconds:259200},
})

const emptyRange = {min:0,max:0}
const veoRouting = (allowReference) => {
  const mode = (intent, min, max) => ({intent,images:{min,max},videos:emptyRange,audios:emptyRange})
  const inputModes = {'text-to-video':mode('text',0,0), 'image-to-video':mode('first_last_frame',1,2),
    ...(allowReference?{'reference-to-video':mode('reference',1,3)}:{})}
  return options(Object.keys(inputModes),undefined,{label:'生成方式',inputModes,
    optionLabels:{'text-to-video':'文生视频','image-to-video':'图生视频','reference-to-video':'参考图生视频'}})
}
for (const id of ['veo3.1-fast','veo3.1-quality']) {
  const fast = id.endsWith('fast')
  add('video',id,{...basic(undefined,res('720p 1080p 4k'),ratio(landscape)),generation_type:veoRouting(fast),official_fallback:officialFallback,enable_gif:flag(undefined,{label:'GIF 动图',visibleWhen:{resolution:'720p'}})},
    [ref('image_urls','image',fast?3:2,{array:true,maxBytes:10000000})],{fixed:{duration:8}})
}
add('video','veo3.1-lite',{...basic(undefined,res('720p 1080p'),ratio(landscape)),enable_gif:flag(undefined,{label:'GIF 动图',visibleWhen:{resolution:'720p'}})},[],{
  ...textVideo,fixed:{duration:8},usage:'仅支持文生视频。',
})
add('video','veo3.1-fast-official veo3.1-quality-official',{
  ...basic(durationChoices([4,6,8],8),res('720p 1080p 4k'),ratio(landscape)),negative_prompt,seed,
  generate_audio:flag(false),enhance_prompt:flag(true),resize_mode:options('pad crop','pad',{label:'素材适配'}),
},frames())

add('video','MiniMax-Hailuo-02',{duration:durationChoices([6,10],6),resolution:res('512p 768p 1080p','768p'),
  prompt_optimizer,fast_pretreatment:flag(false,{label:'快速预处理'}),watermark},frames(),textAndFrames())
add('video','MiniMax-Hailuo-2.3 MiniMax-Hailuo-2.3-Fast',{duration:durationChoices([6,10],6),resolution:res('768p 1080p','768p'),
  prompt_optimizer,fast_pretreatment:flag(false,{label:'快速预处理'}),watermark},frames(false,''),textAndFrames(false))
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
  audio:flag(true),safety_tolerance:number(0,4,2,{label:'安全容差'})}
add('video','flux-3-video',flux,img(10,true),{
  ...imageVideo({textLabel:'文生视频'}),usage:'按顺序连接 1 至 10 张关键帧。',
})
add('video','flux-3-video-edit',{duration:flux.duration,resolution:flux.resolution,audio:flux.audio,safety_tolerance:flux.safety_tolerance},videoOnly('video_url'),{...mention('flux-3-video','FLUX 3 Video · 视频续写',{}),...generationMode('extend','视频续写')})
const fluxDraft = {...flux,resolution:undefined}
const fluxDraftSettings = {upstream:'flux-3-video',fixed:{draft:true,resolution:'hd'}}
add('video','flux-3-video-draft',fluxDraft,img(10,true),{
  ...fluxDraftSettings,...generationMode('first_last_frame','HD 草稿',{manualOnly:true}),name:'FLUX 3 Video · HD 草稿',
})
add('video','flux-3-video-extension-draft',{duration:flux.duration,audio:flux.audio,safety_tolerance:flux.safety_tolerance},videoOnly('video_url'),{
  ...fluxDraftSettings,...generationMode('extend','续写草稿',{manualOnly:true}),name:'FLUX 3 Video · 续写草稿',
})
add('video','flux-3-video-final',{resolution:res('hd fhd','hd'),nsfw_check:undefined},[],{
  upstream:'flux-3-video',name:'FLUX 3 Video · 草稿转正片',hidden:true,promptKey:false,prompt:{label:'原草稿',required:false},
  taskContinuation:{sourceModels:['flux-3-video-draft','flux-3-video-extension-draft'],actionLabel:'草稿转正片'},
})
const sky = {...basic(seconds(3,15),res('480p 720p 1080p','1080p')),prompt_optimizer}
for (const id of ['skyreels-v4-fast','skyreels-v4-std']) {
  const settings = (name, presentation, usage) => ({...mention(id,id+' · '+name,{},usage),...presentation,referenceSyntax:'en-at-numbered'})
  const midFrames = Array.from({length:6},(_,i)=>ref('mid_frame_images','image',1,{role:'mid_frame_'+(i+1),array:true,order:41+i}))
  add('video',id,sky,[...frames(false,'end_frame_image'),...midFrames],settings('图生视频',textAndFrames(),'支持首帧、尾帧和最多 6 个中间帧。'))
  add('video',id+'-reference',sky,[
    ref('ref_images','image',3,{array:true,itemRole:'image'}),
    ref('ref_videos','video',1,{array:true,itemRole:'reference'}),
  ],settings('多模态参考',generationMode('reference','多模态参考生视频',{minInputs:1}),'最多 3 个参考主体和 1 段动作参考视频。'))
  add('video',id+'-grid',sky,[ref('ref_images','image',1,{array:true,itemRole:'grid',min:1})],
    settings('拼贴图驱动',generationMode('reference','拼贴图驱动',{manualOnly:true}),'接入一张带顺序的拼贴图。'))
  add('video',id+'-voice',sky,[
    ref('ref_images','image',1,{array:true,itemRole:'image',min:1,role:'subject_image'}),
    ref('ref_images.audio_url','audio',1,{min:1,role:'source_audio'}),
  ],settings('声纹同步',generationMode('reference','声纹同步'),'接入一张主体图片和一段声纹音频。'))
  add('video',id+'-extend',sky,[ref('ref_videos','video',1,{array:true,itemRole:'extend',min:1,role:'source_video'})],
    settings('视频扩展',generationMode('extend','视频扩展',{manualOnly:true}),'接入一段要扩展的视频。'))
}

const horse = {duration:seconds(3,15),resolution:res('480P 720P 1080P','1080P'),size:ratio(),seed,watermark}
for (const id of ['happyhorse-1.0','happyhorse-1.1']) {
  add('video',id,horse,img(9,true),id==='happyhorse-1.0'?generationMode('reference','参考图生视频',{textLabel:'文生视频'}):{})
  add('video',id+'-frames',horse,frames(true,''),{
    ...mention(id,id+' · 首帧',{}),...(id==='happyhorse-1.0'?generationMode('first_frame','图生视频'):{}),
  })
}
add('video','happyhorse-1.0-edit',{resolution:horse.resolution,audio_setting:options('auto keep mute','auto',{label:'音轨处理'})},[
  ...videoOnly('video_url'),...img(5,true),
],{...mention('happyhorse-1.0','HappyHorse 1.0 · 视频编辑',{}),...generationMode('edit','视频编辑')})

const wan = {duration:seconds(2,15),resolution:res('480P 720P 1080P','1080P'),size:ratio(),negative_prompt,prompt_extend:flag(true),seed,watermark}
const wan3Prompt = {optionalPrompt:true,maxPrompt:20000,referenceSyntax:'cn-short-numbered'}
for (const id of ['wan3.0-video','wan3.0-video-prime']) {
  const fields = {...wan,duration:automaticSeconds(2,30),size:ratio('adaptive '+videoRatios,'adaptive'),audio:flag(true)}
  add('video',id,fields,multimodal(10,5,5),{...wan3Prompt,...textAndReference()})
  add('video',id+'-document',{...fields,prompt_extend:undefined,file_url:publicURL('参考文档网址'),link_url:publicURL('参考网页网址')},multimodal(10,5,5),{
    ...wan3Prompt,...mention(id,id+' · 文档或网页参考',{generation_type:'reference',prompt_extend:true}),
    ...generationMode('reference','文档或网页参考',{manualOnly:true}),
    optionalPrompt:true,usage:'可填写一个公网文档或免登录网页地址，也可搭配参考媒体。',
  })
  add('video',id+'-frames',{...wan,duration:automaticSeconds(2,30),size:undefined,audio:flag(true)},taggedFrames(),{...mention(id,id+' · 首尾帧',{size:'adaptive'},'首帧必填，尾帧可选。'),...imageVideo(),...wan3Prompt})
  add('video',id+'-edit',{resolution:wan.resolution},videoOnly('video_urls'),{...mention(id,id+' · 视频编辑',{generation_type:'edit'}),hidden:true})
}
const drivingAudio = ref('audio_url','audio',1)
const wan27 = {...wan,resolution:res('720P 1080P','1080P')}
add('video','wan2.7',wan27,[...img(2),drivingAudio],textAndFrames(true,{textAudioMax:1,mediaImagesMin:1}))
add('video','wan2.7-extend',{...wan27,size:undefined},videoOnly('video_urls'),{...mention('wan2.7','Wan 2.7 · 视频续写',{}),...generationMode('extend','视频续写')})
add('video','wan2.7-r2v',wan,[
  ref('image_with_roles','image',5,{itemRole:'reference_image',array:true}),video(5),
],{...generationMode('reference','参考生视频',{minInputs:1}),usage:'至少添加一份图片或视频参考，合计最多 5 份。'})
add('video','wan2.7-videoedit',{...wan,duration:durationChoices([0,2,3,4,5,6,7,8,9,10],0),audio_setting:options('auto keep mute','auto',{label:'音轨处理'})},[
  ...img(4),...videoOnly('video_urls'),
],generationMode('edit','视频编辑'))
const wan25 = {duration:durationChoices([5,10],5),resolution:res('480p 720p 1080p'),negative_prompt,prompt_extend:flag(true),seed,watermark,audio:flag(true)}
const audioOrImage = textAndFrames(false,{textAudioMax:1,mediaImagesMin:1})
add('video','wan2.5-preview',{...wan25,size:ratio()},[...img(1,true),drivingAudio],audioOrImage)
const wan26 = {...wan25,duration:durationChoices([5,10,15],5),resolution:res('720p 1080p'),
  shot_type:options('single multi','single',{label:'分镜方式'})}
add('video','wan2.6',{...wan26,aspect_ratio:ratio()},[...img(1),drivingAudio],audioOrImage)
add('video','wan2.6-template',{template:options('squish rotation poke inflate dissolve carousel singleheart flying rose hug frenchkiss coupleheart','squish',{label:'特效模板'}),resolution:wan26.resolution,duration:wan26.duration,watermark},img(1,false,1),{
  ...mention('wan2.6','Wan 2.6 · 图片特效',{}),...generationMode('first_frame','图片特效',{manualOnly:true}),promptKey:false,
})
add('video','wan2.6-i2v',wan26,[...img(1,false,1),drivingAudio],generationMode('first_frame','图生视频'))
add('video','wan2.6-i2v-flash',{...wan26,duration:seconds(2,15),resolution:res('480p 720p 1080p','1080p')},[...img(1,true,1),drivingAudio],generationMode('first_frame','图生视频'))

const kling = {...basic(seconds(3,15),undefined,ratio(smallRatios)),mode:options('std pro 4k','std'),negative_prompt,audio:flag(false),watermark}
const shots = {
  multi_shot:flag(false,{label:'多镜头'}),
  shot_type:options('customize intelligence','customize',{label:'分镜方式',visibleWhen:{multi_shot:true},optionLabels:{customize:'自定义分镜',intelligence:'智能分镜'}}),
  multi_prompt:list(object({prompt:line(512,{label:'镜头提示词',required:true}),duration:line(3,{label:'时长（秒）',required:true,description:'填写 1 秒以上的整数。'})}),{label:'分镜列表',visibleWhen:{multi_shot:true,shot_type:'customize'}}),
}
const klingElements = list(object({
  name:line(128,{label:'主体名称',required:true}),description:line(512,{label:'主体描述',required:true}),
  element_input_urls:list(publicURL('主体图片网址',true),{label:'主体图片网址',required:true,description:'每行一个公网地址，第一张为正面图，共 2 至 4 张。'}),
}),{label:'参考主体',description:'最多 3 个主体，在提示词用 @主体名称 引用。'})
const originalSound = options('no yes','no',{label:'原声',optionLabels:{no:'不保留',yes:'保留'}})
add('video','kling-v2-6',{...kling,mode:options('std pro','std'),duration:durationChoices([5,10],5)},img(2),textAndFrames())
add('video','kling-v3',{...kling,...shots,element_list:klingElements},img(2),{optionalPrompt:true,...textAndFrames()})
for (const id of ['kling-v3-omni','kling-video-o1']) {
  const omni = id==='kling-v3-omni'
  const fields = omni?{...kling,...shots,element_list:klingElements}:{...kling,mode:options('std pro','std'),duration:durationChoices([5,10],5),negative_prompt:undefined,audio:undefined,watermark:undefined}
  const settings = (name, presentation) => ({...mention(id,id+' · '+name,{}),...presentation,optionalPrompt:omni,referenceSyntax:'kling-numbered'})
  add('video',id,fields,img(7),{optionalPrompt:omni,referenceSyntax:'kling-numbered',...textAndImageReference()})
  add('video',id+'-frames',fields,taggedFrames(),settings('首尾帧',imageVideo()))
  for (const [intent,label,referType] of [['reference','视频特征参考','feature'],['edit','视频编辑','base']]) {
    add('video',id+'-'+referType,{...fields,keep_original_sound:originalSound},[...img(7),ref('video_list','video',1,{array:true,itemRole:referType,min:1})],
      settings(label,generationMode(intent,label,{manualOnly:intent==='edit'})))
  }
}
add('video','kling-3.0-turbo',{...basic(seconds(3,15),res('720p 1080p'),ratio(smallRatios)),watermark},frames(false,''),textAndFrames(false))
add('video','kling-v2-6-motion-control kling-v3-motion-control',{
  mode:options('std pro','std'),character_orientation:options('image video','image',{label:'人物朝向'}),
  keep_original_sound:options('yes no','yes',{label:'保留原声'}),
},[ref('image_url','image',1,{min:1,role:'first_frame'}),...videoOnly('video_url')],{optionalPrompt:true})
for (const id of ['kling-v4','kling-v4-flash']) {
  const flash = id.endsWith('flash')
  const fields = basic(seconds(3,flash?20:30),flash?undefined:res('720p 1080p'),ratio(smallRatios+' 21:9'))
  add('video',id,fields,[],{...textVideo,fixed:flash?{resolution:'720p'}:undefined})
  const fixed = flash ? {resolution:'720p'} : {}
  const audioSetting = options('auto origin',undefined,{label:'参考视频原声',optionLabels:{auto:'重新配声',origin:'保留输入原声'}})
  add('video',id+'-frames',{...fields,aspect_ratio:undefined},frames(true,flash?'':'last_frame_image'),{...mention(id,id+(flash?' · 首帧':' · 首尾帧'),fixed),...generationMode(flash?'first_frame':'first_last_frame','图生视频'),
    usage:flash?'仅接入一张首帧。':'接入首帧，可加尾帧。'})
  add('video',id+'-reference',{...fields,aspect_ratio:ratio('auto '+smallRatios+' 21:9','auto'),audio_setting:audioSetting},multimodal(10,5,flash?0:3),{
    ...mention(id,id+' · 多模态参考',{...fixed,omni_reference_task_type:'reference'}),
    ...generationMode('reference',flash?'参考生视频':'多模态参考生视频',{minInputs:1}),referenceSyntax:'en-at-numbered',
  })
  for (const [mode,label] of [['edit','编辑'],['extend','续写']]) {
    add('video',id+'-'+mode,{...fields,duration:mode==='edit'?number(3,flash?20:30):fields.duration,audio_setting:mode==='edit'&&!flash?audioSetting:undefined},
      [...videoOnly('video_url'),...(mode==='edit'?img(10):[])],{
        ...mention(id,id+' · 视频'+label,{...fixed,omni_reference_task_type:mode}),
        ...(mode==='edit'?editVideo:generationMode('extend','视频续写',{manualOnly:true})),referenceSyntax:'en-at-numbered',
      })
  }
}

const vidu = {...basic(seconds(1,16),res('540p 720p 1080p')),seed,audio:flag(true)}
add('video','viduq3-pro viduq3-turbo',vidu,img(2),textAndFrames())
add('video','viduq3',{...vidu,duration:seconds(3,16),audio:undefined},img(7,false,1),generationMode('reference','参考生视频'))
add('video','viduq3-mix',{...vidu,resolution:res('720p 1080p'),audio:undefined},img(7,false,1),generationMode('reference','参考生视频'))
const q4 = {...vidu,duration:seconds(3,16),resolution:res('540p 720p 1080p 2K 4K')}
add('video','viduq4-preview',q4,[],{hidden:true,name:'Vidu Q4 Preview',usage:'请选择图生视频或参考生视频。'})
add('video','viduq4-preview-frames',{...q4,aspect_ratio:undefined},frames(true,''),{
  ...mention('viduq4-preview','Vidu Q4 · 首帧',{},'仅接入一张首帧。'),...generationMode('first_frame','图生视频'),optionalPrompt:true,
})
add('video','viduq4-preview-reference',q4,[
  ref('image_with_roles','image',15,{itemRole:'reference_image',array:true,min:1,inline:true}),audio(3),
],{...mention('viduq4-preview','Vidu Q4 · 素材参考',{},'至少一张参考图，可搭配参考音频。'),
  ...generationMode('reference','参考生视频')})

for (const id of ['grok-imagine-1.0-video-apimart','grok-imagine-1.5-video-apimart']) {
  const is15 = id.includes('1.5')
  const upstream = is15?'grok-imagine-1.5-video-ext':id
  const name = is15?'Grok Imagine 1.5 Video':id
  const fields = {duration:seconds(1,15,6),resolution:res('480p 720p 1080p','480p'),size:ratio(smallRatios)}
  const referenceFields = {...fields,resolution:res('480p 720p','480p')}
  const settings = (label,presentation) => ({...mention(upstream,name+' · '+label,{}),...presentation})
  add('video',id,fields,[],settings('文生视频',textVideo))
  add('video',id+'-reference',referenceFields,img(14,false,1),settings('参考图生视频',generationMode('reference','参考图生视频')))
  add('video',id+'-frames',fields,taggedFrames(),settings('图生视频',imageVideo()))
  if (is15) {
    add('video',id+'-frame-reference',referenceFields,[
      ...taggedFrames(),ref('image_with_roles','image',14,{array:true,itemRole:'reference_image',min:1}),
    ],settings('首帧加参考图',generationMode('reference','首帧加参考图',{manualOnly:true})))
  }
}
for (const id of ['grok-imagine-video','grok-imagine-video-1.5']) {
  const fields = basic(seconds(1,15,8),res(id.endsWith('1.5')?'480p 720p 1080p':'480p 720p','480p'),ratio(videoRatios+' 2:3 3:2'))
  add('video',id,fields,img(id.endsWith('1.5')?7:1))
  add('video',id+'-edit',{resolution:fields.resolution},videoOnly('video.url'),mention(id,id+' · 视频编辑',{}))
}

const pix = {duration:seconds(1,15),resolution:res('360p 540p 720p 1080p','540p'),audio:flag(false),seed:number(0,4294967295)}
const pixFields = {...pix,size:ratio(),negative_prompt:line(2048,{control:'textarea'}),watermark}
const multiClip = flag(undefined,{label:'连续多片段'})
add('video','pixverse-v6',{...pixFields,generate_multi_clip_switch:multiClip},frames(),{
  maxPrompt:5000,fixed:{motion_mode:'normal'},
  ...generationMode('first_last_frame','首尾帧过渡',{textLabel:'文生视频',mediaImagesMin:2}),usage:'按首帧和尾帧生成过渡。',
})
add('video','pixverse-v6-i2v',{...pixFields,generate_multi_clip_switch:multiClip},img(1,false,1),{
  maxPrompt:5000,
  ...mention('pixverse-v6','PixVerse V6 · 图生视频',{motion_mode:'normal'}),...generationMode('first_frame','单图生视频'),
})
add('video','pixverse-v6-extend',pixFields,[],{
  ...mention('pixverse-v6','PixVerse V6 · 视频续写',{motion_mode:'normal'}),...continueTask(['pixverse-v6','pixverse-v6-i2v','pixverse-v6-reference']),
})
add('video','pixverse-v6-reference',pixFields,[ref('img_references','image',7,{array:true,min:1})],{
  ...mention('pixverse-v6','PixVerse V6 · 多图融合',{motion_mode:'normal'}),...generationMode('reference','多参考图融合'),
})
add('video','pixverse-v6-max',{...pix,aspect_ratio:ratio(videoRatios+' 2:3 3:2')},img(1),textAndFrames(false))
add('video','muse-video-ext',{duration:seconds(2,10,10),aspect_ratio:options(videoRatios+' 2:3 3:2'),language:options('zh en')},img(9,true),{fixed:{resolution:'720p',n:1},...textAndImageReference()})

const omni = {duration:seconds(3,15,10),resolution:res('360p 720p 1080p 4k'),aspect_ratio:ratio(landscape)}
const omniExtID = 'gemini-omni-1.1-flash-ext'
const omniExtFields = {...omni,duration:durationChoices([4,6,8,10],6)}
add('video','Omni-Flash-Ext',omniExtFields,[...img(3),video(1)],{
  upstream:omniExtID,name:'Gemini Omni 1.1 Flash Ext',...textAndReference(),usage:'可接入 1 或 3 张参考图及 1 段参考视频。',
})
add('video','Omni-Flash-Ext-frames',omniExtFields,img(1,false,1),{
  ...mention(omniExtID,'Gemini Omni 1.1 Flash Ext · 首帧',{generation_type:'frame'}),...generationMode('first_frame','图生视频'),
})
for (const id of ['gemini-omni-1.1-flash','gemini-omni-1.2-flash','gemini-omni-1.2-flash-ext']) {
  const promptControls = id==='gemini-omni-1.2-flash-ext' ? {negative_prompt:line(12000,{control:'textarea'}),system_prompt:line(12000,{label:'系统指令',control:'textarea'})} : {}
  const fields = {...omni,...promptControls,duration:id.includes('1.1')?undefined:omni.duration}
  const wireTask = task => id.includes('1.1')?{metadata:{task}}:{omni_reference_task_type:task}
  add('video',id,fields,img(10),textAndImageReference('多图参考生视频',{mediaImagesMin:id.endsWith('-ext')?1:2}))
  add('video',id+'-frames',{...fields,negative_prompt:undefined,system_prompt:undefined,duration:fields.duration?seconds(3,10,10):undefined},frames(true),{...mention(id,id+' · 首尾帧',{}),...imageVideo()})
  for (const [mode,label] of [['edit','编辑'],['extend','续写']]) {
    add('video',id+'-'+mode,{...promptControls,resolution:fields.resolution,duration:mode==='extend'&&fields.duration?seconds(3,10,10):undefined},
      videoOnly('video_urls'),{
        ...mention(id,id+' · 视频'+label,wireTask(mode)),
        ...generationMode(mode,'视频'+label,{manualOnly:mode==='extend'}),
      })
  }
  add('video',id+'-task-extend',{...promptControls,resolution:fields.resolution,duration:fields.duration?seconds(3,10,10):undefined},[],{
    ...mention(id,id+' · 按原任务续写',wireTask('extend')),...continueTask([id,id+'-frames',id+'-edit',id+'-extend']),
  })
}
const omniPreview = {aspect_ratio:ratio(landscape)}
add('video','gemini-omni-flash-preview',omniPreview,[...img(16),video(1)],{fixed:{resolution:'720p'},...textAndReference()})
add('video','gemini-omni-flash-preview-task-extend',omniPreview,[],{
  ...mention('gemini-omni-flash-preview','Gemini Omni Flash · 按原任务续写',{resolution:'720p'}),...continueTask(['gemini-omni-flash-preview']),
})
