// SPDX-License-Identifier: Apache-2.0
import { add, decimal, flag, generationMode, line, number, options, ref, sunoVersions, sunoModelVersions, sunoCustomModelID } from './definitions.mjs'

const flowFields = {
  lyrics:line(5000,{control:'textarea',label:'歌词'}),title:line(120),
  bpm:number(1,undefined,120,{label:'每分钟节拍'}),length:number(1,240,60,{label:'时长（秒）',control:'duration'}),
  seed:line(32),
}
add('music','flowmusic',flowFields,[],{...generationMode('text','音乐生成'),name:'Flow Music',promptKey:'sound_prompt',optionalPrompt:true,action:'generate',usage:'填写风格提示词或歌词。'})
add('music','flowmusic-lyria-3.5',flowFields,[],{
  upstream:'flowmusic',...generationMode('text','Lyria 3.5',{manualOnly:true}),name:'Flow Music · Lyria 3.5',promptKey:'sound_prompt',optionalPrompt:true,
  fixed:{version:'lyria-3.5'},action:'generate-lyria-3.5',usage:'填写风格提示词或歌词。',
})
const audioFormat = options('mp3 m4a wav','mp3')
add('music','suno-sounds',{
  version:sunoVersions,audio_format:audioFormat,
  type:options('one-shot loop','one-shot',{label:'音效类型',optionLabels:{'one-shot':'单次',loop:'循环'}}),
  bpm:number(1,300,undefined,{label:'每分钟节拍'}),key:line(24,{label:'调性'}),
},[],{upstream:'suno',...generationMode('text','生成音效',{manualOnly:true}),name:'Suno V6 · 生成音效',endpoint:'/v1/music/generations/sounds',action:'sounds'})

const sunoStyle = {
  version:sunoModelVersions,custom_model_id:sunoCustomModelID,title:line(80),audio_format:audioFormat,auto_lyrics:flag(undefined),
  tags:line(1000,{control:'textarea',label:'风格标签'}),negative_tags:line(1000),
  style_weight:decimal(0,1,undefined,{step:0.01,control:'slider'}),audio_weight:decimal(0,1,undefined,{step:0.01,control:'slider'}),
  weirdness:decimal(0,1,undefined,{step:0.01,control:'slider',label:'创意度'}),
  vocal_gender:options('Male Female'),variety:options('off normal high extra max'),max_mode:flag(false),
}
const persona = line(128,{visibleWhenAny:[{version:'v6'},{version:'v6-wild'},{version:'v6-mini'}]})
const coverCustom = {custom:'custom'}
const coverStyle = Object.fromEntries(Object.entries(sunoStyle).map(([key, field]) => [key,
  ['version','custom_model_id','audio_format'].includes(key) ? field : {...field,visibleWhen:coverCustom},
]))
const oneAudio = [ref('audio_url','audio',1,{min:1,role:'source_audio'})]
const sunoAction = (path, action, name, intent, manualOnly = false) => ({
  ...generationMode(intent,name,{manualOnly}),
  upstream:'suno',name:'Suno V6 · '+name,endpoint:'/v1/music/generations/'+path,musicTask:true,action,optionalPrompt:true,maxPrompt:5000,
})
add('music','suno-inspiration',sunoStyle,[ref('audio_urls','audio',4,{array:true,min:1})],sunoAction('inspo','inspo','参考音频创作','reference'))
add('music','suno-upload-cover',{...coverStyle,custom:options('custom inspiration','custom',{optionLabels:{custom:'自定义',inspiration:'灵感'}}),instrumental:flag(false),
  gpt_description:line(3000,{label:'灵感描述',control:'textarea',required:true,visibleWhen:{custom:'inspiration'}}),
  persona_id:{...persona,visibleWhen:coverCustom},duration_s:number(10,360,undefined,{label:'时长（秒）',control:'duration',visibleWhen:coverCustom})},
  oneAudio,sunoAction('uploadCover','upload_cover','翻唱','edit',true))
add('music','suno-upload-extend',{...sunoStyle,persona_id:persona,continue_at:decimal(0,undefined,undefined,{label:'续写起点（秒）',required:true}),
  duration_s:number(10,360,undefined,{label:'新增时长（秒）',control:'duration'})},oneAudio,sunoAction('uploadExtend','upload_extend','音乐续写','extend',true))

const operationIs = operation => ({operation})
const only = (field, operation) => ({...field,visibleWhen:operationIs(operation)})
const followUpAudio = options("mp3 wav","mp3")
const sourceMusic = (id, label, sources) => ({
  upstream:id,name:label,hidden:true,promptKey:false,prompt:{required:false,label:"原音乐"},
  taskContinuation:{sourceModels:sources,actionLabel:label},usage:"沿用选中的原音轨，设置操作后确认生成。",
})
const interval = {
  start_s:only(decimal(0,undefined,undefined,{label:"替换起点（秒）",required:true}),"replace"),
  end_s:only(decimal(0,undefined,undefined,{label:"替换终点（秒）",required:true}),"replace"),
}
const actions = {extend:"续写",cover:"翻唱",replace:"替换片段",stem:"提取单轨",stems:"提取全部分轨"}
const sunoTextOps = ["extend","cover","replace"].map(operationIs)
const vocalOps = ["extend","cover"].map(operationIs)
const asCustom = ["extend","cover"].map(operation=>({operation,custom:"custom"}))
const asInspiration = ["extend","cover"].map(operation=>({operation,custom:"inspiration"}))
const customStyle = Object.fromEntries(["title","tags","negative_tags","style_weight","weirdness","audio_weight"].map(key=>[key,{...sunoStyle[key],visibleWhenAny:["title","tags","negative_tags"].includes(key)?[...asCustom,operationIs("replace")]:asCustom}]))
add("music","suno-follow-up",{
  operation:options("extend cover replace stem stems","extend",{label:"续作方式",section:"common",optionLabels:actions}),
  version:{...sunoModelVersions,visibleWhenAny:sunoTextOps},audio_format:followUpAudio,
  custom_model_id:{...sunoCustomModelID,visibleWhenAny:sunoTextOps.map(rule=>({...rule,version:"custom"}))},
  custom:options("custom inspiration","custom",{label:"创作方式",visibleWhenAny:vocalOps,optionLabels:{custom:"自定义",inspiration:"灵感"}}),
  prompt:line(5000,{label:"歌词或修改要求",control:"textarea",visibleWhenAny:[...asCustom,operationIs("replace")]}),
  gpt_description:line(3000,{label:"灵感描述",control:"textarea",visibleWhenAny:asInspiration}),
  continue_at:only(decimal(0,undefined,undefined,{label:"续写起点（秒）",required:true}),"extend"),
  duration_s:number(10,360,undefined,{label:"目标时长（秒）",visibleWhenAny:asCustom}),
  auto_lyrics:only(flag(undefined),"extend"),
  variety:{...sunoStyle.variety,visibleWhenAny:sunoTextOps},
  vocal_gender:{...sunoStyle.vocal_gender,visibleWhenAny:vocalOps},
  max_mode:{...sunoStyle.max_mode,visibleWhenAny:[...asCustom,operationIs("replace")]},
  persona_id:line(128,{label:"Persona ID",visibleWhenAny:asCustom.flatMap(rule=>["v6","v6-wild","v6-mini"].map(version=>({...rule,version})))}),
  ...customStyle,...interval,infill_lyrics:only(line(5000,{label:"片段歌词",control:"textarea"}),"replace"),
  stem_type:only(options("lead_vocal drum_kit bass backing_vocals piano electric_guitar percussion string_section synth acoustic_guitar sound_effects synth_pad synth_bass guitar brass_section organ electronic_drum_kit lead_electric_guitar synth_keys rhythm_electric_guitar electric_piano upright_bass keyboards distorted_electric_guitar kick snare risers synth_strings synth_lead woodwinds flute harp tambourine trumpet arpeggiator accordion fiddle pedal_steel_guitar synth_voice violin digital_piano synth_brass mandolin choir banjo bells clarinet tenor_saxophone trombone shaker french_horn glockenspiel electric_bass cello timpani harmonica marimba vibraphone lap_steel_guitar saxophone orchestra horns cymbals hand_clap oboe celesta congas drone alto_saxophone double_bass ukulele harpsichord baritone_saxophone xylophone tuba bass_guitar whistle lead_guitar rhodes 808 bongos bassoon cowbell viola sitar steel_drums piccolo theremin bagpipes hi_hat music_box melodica tabla koto djembe taiko didgeridoo","lead_vocal",{label:"分轨类型"}),"stem"),
},[],sourceMusic("suno","Suno 音乐续作",["suno","suno-inspiration","suno-upload-cover","suno-upload-extend"]))
add("music","flowmusic-follow-up",{
  operation:options("extend cover replace stems","extend",{label:"续作方式",section:"common",optionLabels:actions}),
  instruction:line(5000,{label:"音乐编辑指令",control:"textarea",required:true,visibleWhenAny:["extend","cover","replace"].map(operationIs)}),
  title:line(120,{visibleWhenAny:["extend","cover","replace"].map(operationIs)}),
  extend_from_s:only(decimal(0,undefined,undefined,{label:"续写起点（秒）",required:true}),"extend"),
  extend_s:only(decimal(0.01,164,undefined,{label:"续写时长（秒）",required:true}),"extend"),
  strength:only(decimal(0,1,undefined,{label:"改编强度",required:true}),"cover"),
  ...interval,seed:line(32,{visibleWhenAny:["extend","cover"].map(operationIs)}),
  replacement_seed:only(number(undefined,undefined,undefined,{label:"随机种子"}),"replace"),
},[],{...sourceMusic("flowmusic","Flow Music 音乐续作",["flowmusic","flowmusic-lyria-3.5"]),outputKinds:["audio","audio-archive"]})
add('speech','gpt-audio-mini',{
  voice:options('alloy ash ballad coral echo sage shimmer verse','alloy'),
  response_format:options('wav mp3','wav'),
},[],{
  name:'GPT Audio Mini',maxPrompt:4096,endpoint:'/v1/chat/completions',defaultDisabled:true,
  usage:'生成语音回复。使用兼容 Chat Completions 的音频输出参数，启用前需核对账号兼容性。',
})
