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

add('speech','gpt-audio-mini',{
  voice:options('alloy ash ballad coral echo sage shimmer verse','alloy'),
  response_format:options('wav mp3','wav'),
},[],{
  name:'GPT Audio Mini',maxPrompt:4096,endpoint:'/v1/chat/completions',defaultDisabled:true,
  usage:'生成语音回复。使用兼容 Chat Completions 的音频输出参数，启用前需核对账号兼容性。',
})
