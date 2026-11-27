// SPDX-License-Identifier: Apache-2.0

export const ratios = '1:1 2:3 3:2 3:4 4:3 4:5 5:4 9:16 16:9 21:9'
export const wideRatios = ratios + ' 1:4 4:1 1:8 8:1'
export const videoRatios = '16:9 9:16 1:1 4:3 3:4 21:9'
export const common = {
  aspect_ratio: '宽高比', size: '画面尺寸', resolution: '分辨率', duration: '时长（秒）',
  n: '生成数量', quality: '质量', seed: '随机种子', negative_prompt: '负面提示词',
  generate_audio: '生成声音', audio: '生成声音', watermark: '水印', mode: '档位',
  output_format: '输出格式', output_compression: '压缩质量', version: '版本',
  prompt_extend: '扩写提示词', prompt_optimizer: '优化提示词', enhance_prompt: '增强提示词',
  width: '宽度（像素）', height: '高度（像素）', background: '背景', moderation: '审核强度',
  sequential_image_generation: '组图模式', enable_sequential: '组图模式',
  auto_aspect_ratio: '自适应比例', web_grounding: '联网增强', grounding: '联网增强',
  google_search: 'Google 搜索', google_image_search: 'Google 图片搜索',
  style: '风格', vocal_gender: '人声', style_weight: '风格权重', weirdness_constraint: '创意度',
  audio_weight: '音频权重', instrumental: '纯音乐', audio_format: '音频格式', title: '曲名',
  custom: '生成方式', max_mode: 'Max 模式', custom_model_id: '自定义模型 ID',
  persona_id: 'Persona ID', negative_tags: '排除风格', auto_lyrics: '改写歌词', variety: '风格变化',
  speed: '速度', voice: '音色', response_format: '文件格式', language: '语言',
}

export const options = (choices, value, extra = {}) => {
  const values = typeof choices === 'string' ? choices.split(' ') : choices
  return {type: typeof values[0] === 'number' ? 'integer' : 'string', enum: values, default: value, ...extra}
}
export const number = (minimum, maximum, value, extra = {}) => ({type: 'integer', minimum, maximum, default: value, ...extra})
export const decimal = (minimum, maximum, value, extra = {}) => ({type: 'number', minimum, maximum, default: value, ...extra})
export const flag = (value, extra = {}) => ({type: 'boolean', default: value, ...extra})
export const line = (maxLength, extra = {}) => ({type: 'string', maxLength, ...extra})
export const ref = (field, kind, max, extra = {}) => ({field, kind, max, array: max > 1, ...extra})
export const images = (max, inline = true, field = 'image_urls') => [ref(field, 'image', max, {array: true, inline})]
export const frames = (required = false, end = 'last_frame_image', first = 'first_frame_image') => [
  ref(first, 'image', 1, {role: 'first_frame', min: Number(required)}),
  ...(end ? [ref(end, 'image', 1, {role: 'last_frame'})] : []),
]

const model = (kind, id, fields, references, settings) => {
  const {name, maxPrompt, optionalPrompt, usage, endpoint, upstream, fixed, musicTask, promptKey, hidden, transcription, action, ...rest} = settings
  const capabilities = {image: 'image.generate', video: 'video.generate', music: 'music.generate', speech: 'speech.generate', transcription: 'speech.transcribe'}
  const route = {image: '/v1/images/generations', video: '/v1/videos/generations', music: '/v1/music/generations', speech: '/v1/audio/speech', transcription: '/v1/audio/transcriptions'}
  const isNamed = references.some(r => r.role)
  const roleLabels = {first_frame: '首帧', last_frame: '尾帧', mask: '重绘蒙版', source_video: '源视频', source_audio: '源音频', style_image: '风格图', subject_image: '主体图'}
  const inputRoles = isNamed ? Object.fromEntries(references.flatMap((r, group) =>
    Array.from({length: r.max}, (_, index) => {
      const key = r.role || (r.field.replaceAll('_', '-') + '-reference-' + (index + 1))
      r.roles ??= []
      r.roles[index] = key
      return [key, {kind: r.kind, label: roleLabels[key] || `参考${{image: '图', video: '视频', audio: '音频'}[r.kind]} ${index + 1}`, order: group * 20 + index + 1, required: index < (r.min || 0)}]
    })
  )) : undefined
  const display = Object.fromEntries(Object.entries(fields).filter(([, f]) => f).map(([key, f]) => {
    const choices = f.enum || []
    const control = ['aspect_ratio', 'size'].includes(key) && choices.some(v => typeof v === 'string' && v.includes(':')) ? 'ratio'
      : key === 'resolution' && choices.length ? 'resolution'
      : key === 'duration' && f.type === 'integer' ? 'duration'
      : key === 'n' && f.type === 'integer' ? 'count'
      : key === 'seed' && f.type === 'integer' ? 'seed' : undefined
    const section = ['duration', 'aspect_ratio', 'size', 'resolution', 'quality', 'n', 'mode', 'custom', 'version', 'instrumental', 'voice', 'title'].includes(key) ? 'common' : 'advanced'
    return [key, {label: common[key] || key.replaceAll('_', ' '), section, control, ...f}]
  }))
  const promptLabels = {image: '画面提示词', video: '镜头提示词', music: '音乐提示词', speech: '配音文本'}
  return {
    model: {
      id, label: name || id, capability: capabilities[kind], fields: display,
      inputKinds: [...new Set(references.map(r => r.kind))],
      maxInputs: references.reduce((sum, r) => sum + r.max, 0), inputRoles,
      prompt: kind !== 'transcription' && promptKey !== false ? {required: !optionalPrompt, label: promptLabels[kind], maxLength: maxPrompt} : undefined,
      usageNote: usage, hidden, transcription, ...rest,
    },
    upstream, references, endpoint: endpoint || route[kind], fixed,
    promptKey: promptKey === false ? undefined : (promptKey || (kind !== 'transcription' ? (kind === 'speech' ? 'input' : 'prompt') : undefined)),
    musicTask: musicTask ?? (kind === 'music'), action,
  }
}

export const entries = []
export const add = (kind, ids, fields = {}, refs = [], settings = {}) => {
  for (const id of ids.split(' ')) entries.push(model(kind, id, fields, structuredClone(refs), settings))
}

export const object = (properties, extra = {}) => ({type: 'object', properties, ...extra})
export const list = (items, extra = {}) => ({type: 'array', items, ...extra})

const nano21 = {
  size: options('auto ' + wideRatios, 'auto'), resolution: options('1K 2K 4K', '1K'),
}
add('image', 'gemini-nano-banana-2.1', {...nano21, n: number(1, 4, 1)}, [ref('image_urls','image',14,{inline:true,maxBytes:20000000})], {
  name: 'Nano banana 2.1 · 官方版', usage: '接受参考图做编辑。官方版按实际 Token 结算。可一次生成 1–4 张。',
})
add('image', 'gemini-nano-banana-2.1-ext', nano21, [ref('image_urls','image',14,{inline:true,maxBytes:40000000})], {
  name: 'Nano banana 2.1 · Ext', usage: '接受参考图做编辑。每次生成 1 张，按分辨率计价。', fixed: {n: 1},
})

const magi = {
  duration: number(4, 30, 5), aspect_ratio: options(videoRatios, '16:9'),
  generate_audio: flag(true), seed: number(-1, 2147483647, -1),
}
const magiSettings = {upstream: 'magi-2', maxPrompt: 15000, fixed: {resolution: '1080p'}}
add('video', 'magi-2', magi, [], {...magiSettings, name: 'MAGI-2 · 文生视频', usage: '固定 1080p。描述镜头、动作与声音。'})
add('video', 'magi-2-frames', {...magi, aspect_ratio: options('auto 16:9 9:16 1:1 4:3 3:4','auto')}, frames(true), {
  ...magiSettings, name: 'MAGI-2 · 首尾帧', usage: '首帧必填，尾帧可选。自动比例跟随首帧。固定 1080p。',
})
add('video', 'magi-2-reference', magi, [...images(10, false), ref('video_urls', 'video', 5), ref('audio_urls', 'audio', 5)], {
  ...magiSettings, name: 'MAGI-2 · 多模态参考', usage: '至少添加一项参考。参考视频和音频各最多五段，每类总时长建议不超过 30 秒。',
  fixed: {...magiSettings.fixed, omni_reference_task_type: 'reference'},
})

export const sunoVersions = options('v6 v6-wild v6-mini', 'v6', {
  optionLabels: {'v6': 'V6', 'v6-wild': 'V6 Wild', 'v6-mini': 'V6 Mini'},
})
const customOnly = {visibleWhen: {custom: 'custom'}}
add('music', 'suno', {
  custom: options('inspiration custom', 'inspiration', {
    optionLabels: {inspiration: '灵感', custom: '自定义'},
  }), version: options('v6 v6-wild v6-mini custom', 'v6', {
    optionLabels: {...sunoVersions.optionLabels,custom:'自定义模型'},
  }), instrumental: flag(false),
  title: line(80, {visibleWhen: {custom: 'custom'}}), style: line(1000, {visibleWhen: {custom: 'custom'}, control: 'textarea'}),
  negative_tags: line(1000, customOnly), auto_lyrics: flag(undefined, customOnly),
  persona_id: line(128,{visibleWhen:{custom:'custom'},visibleWhenAny:[{version:'v6'},{version:'v6-wild'},{version:'v6-mini'}]}),
  custom_model_id: line(36,{required:true,visibleWhen:{version:'custom'}}),
  vocal_gender: options('Male Female', undefined, {optionLabels: {Male: '男声', Female: '女声'}}),
  style_weight: decimal(0, 1, undefined, {step: 0.01, control: 'slider'}),
  weirdness_constraint: decimal(0, 1, undefined, {step: 0.01, control: 'slider'}),
  audio_weight: decimal(0, 1, undefined, {step: 0.01, control: 'slider'}),
  variety: options('off normal high extra max'), max_mode: flag(false, customOnly),
  audio_format: options('mp3 m4a wav', 'mp3'), duration: number(10, 360, undefined, customOnly),
}, [], {
  name: 'Suno V6 · 生成音乐', optionalPrompt: true, maxPrompt: 5000, action: 'music',
  usage: '灵感模式输入歌曲设想。自定义模式输入歌词、风格和曲名，纯音乐可省略歌词。默认返回 MP3。',
})

add('speech', 'gpt-4o-mini-tts', {
  voice: options('alloy echo fable onyx nova shimmer', 'alloy'),
  response_format: options('wav opus aac flac', 'wav'),
  speed: decimal(0.25, 4, 1, {step: 0.05}),
}, [], {name: 'GPT-4o Mini TTS', maxPrompt: 4096})

add('transcription', 'whisper-1', {language: line(35)}, [ref('file', 'audio', 1, {min: 1})], {
  name: 'Whisper-1', hidden: true, transcription: {maxBytes: 25000000, maxDurationMs: 10800000, languages: [], wordTimestamps: false, speakerDiarization: false},
})
