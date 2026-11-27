# 媒体生成能力与续作来源

下表核对 APIMart 1.3.10、MiniMax 2.2.6、即梦／火山方舟 2.2.2 候选的画布入口。每个模型的输入数量、角色、规格和价格继续由对应模型声明与渠道预检提供。

| 渠道 | 生成与素材处理 | 原任务操作 |
| --- | --- | --- |
| APIMart 图片 | Nano banana、GPT Image、Seedream、FLUX、Kling、Grok、Muse 等。按具体模型提供文生图、参考图、多图、蒙版、标注或图层。 | Midjourney 从原网格执行变体、放大、扩图与重抽，合格的单图可用遮罩重绘。Grok Imagine 2.0 Image Ext 从同渠道已完成单图做选区编辑。 |
| APIMart 视频 | H3、Seedance、MAGI-2、Zoe、可灵、SkyReels、Wan、FLUX 3、Veo、Gemini Omni 等按具体模式提供文生、图生、多模态参考、视频编辑或素材续写。 | Seedance 2.5、Zoe 和 FLUX 3 Video 用原样片转正。标准 H3 用原 768P 成片升为 2K。Veo、PixVerse 和相应 Gemini Omni 版本可沿用原任务续写。 |
| APIMart 音频 | Suno 和 Flow Music 创作音乐，Suno 提供上传翻唱和续写。Whisper 提供转录。 | Suno 和 Flow Music 依据指定版本对应的原音轨续写、翻唱、替换或分轨，具体选项随模型而定。 |
| MiniMax Design／MiniMax | H3 Image／Highspeed、Design 的 Nano banana 2／Pro 和 Midjourney 生图。标准 H3 接收参考图、视频和音频。H3 Highspeed、Lite、Max 的当前插件模式使用首尾帧和图片参考。另有 MiniMax Music、ElevenLabs Music、TTS、音色设计和转录。 | 标准 H3 的有效 768P 样片可升 2K。官方 ID 方式核对原官方账号。Design 路径先按原账号读取成片与参考，再走官方 `base_video`，随原提示词重建原参考图、视频和音频。 |
| 即梦／火山方舟 | Seedream 4.x、5.0 Lite／Pro／Flash、5.5 生图与图生图，部分款有组图、交互编辑和图层。Seedance 2.5／Flash、2.0／Fast／Mini 支持视频参考、编辑和延长。2.5 在 API 中有显式任务类型，2.0 由提示词引导任务意图。 | Seedance 2.5 草稿沿用原样片、账号及原 CLI 或 API 渠道。当前插件生成 1080p 正片，按各渠道原票据时效校验。 |

参考 [APIMart API](https://docs.apimart.ai/llms.txt)、[MiniMax H3 再生成](https://platform.minimax.cn/docs/api-reference/video-generation-v2-regeneration)、[方舟 2.5 样片模式](https://docs.volcengine.com/docs/ark/seedance-2-5?lang=zh)、[方舟 2.0 编辑与延长](https://docs.volcengine.com/docs/ark/seedance-2-0?lang=zh)。

## 原任务与素材

从结果进入「草稿转正」「升为 2K」「按原任务续写」或指定图片编辑时，宿主使用所选版本的受信回执与已应用的创建命令。客户端提交的来源需与节点绑定一致。APIMart 的任务来源需要原渠道、模型、凭据、素材版本和有效期。原任务节点的媒体连线记录上游来源，特定遮罩仍需满足原图尺寸和权限。切换模型或换一条连线无法替换受信任务 ID。

「用视频续写」「用音频续写」固定所选版本的素材。只要画布有权使用该素材、目标模型接受其种类和尺寸／时长，就可以使用其他渠道产出的文件。目标插件按其接口取得素材 URL 或上传文件。该流程沿用普通生成约束。

Veo 3.1 Fast／Quality Ext 的原任务续写需要相同模型。开启官方兜底后，通用查询结果无法证明实际走过哪条渠道。只有记录了关闭官方兜底的原回执会提供该按钮。此前缺少这个字段的回执同样无法用于该按钮。[Veo 生成与兜底](https://docs.apimart.ai/cn/api-reference/videos/veo3/generation.md) · [Veo 续写](https://docs.apimart.ai/cn/api-reference/videos/veo3/remix.md)

## APIMart Ext 和对应版本

Ext 是 APIMart 为各家模型定义的入口或别名，逐款决定参数限制、输出和价格。

| 型号 | Ext 和对应版本的差异 |
| --- | --- |
| Nano banana 2.1 | 官方版和 Ext 输出档位为 1K、2K、4K。官方版一次 1–4 张，按实际 Token 计费。Ext 一次一张，按分辨率、实际出图张数计费。开启官方兜底并实际使用官方版时按官方计费。[接口](https://docs.apimart.ai/cn/api-reference/images/gemini-nano-banana-2.1/generation.md) |
| Nano banana／Pro／2 和 Lite | 普通、Pro、2 的 Ext 入口一次一张，可开启官方兜底。Lite 与 Lite Ext 参数相同，固定 1K，使用各自渠道价格，均无官方兜底参数。[Nano banana](https://docs.apimart.ai/cn/api-reference/images/gemini-2.5-flash/generation.md) · [Pro](https://docs.apimart.ai/cn/api-reference/images/gemini-3-pro/generation.md) · [Nano banana 2](https://docs.apimart.ai/cn/api-reference/images/gemini-3.1-flash/generation.md) · [Lite](https://docs.apimart.ai/cn/api-reference/images/gemini-3.1-flash/generation-lite.md) |
| GPT Image 2／2.5／2.6 | 2 Ext 一次一张，可兜底。2 官方版支持透明 PNG 蒙版和多图。2.5／2.6 Ext 在参数中选择 Flare 或 Sunburst，按张计费。2.5 Ext 一次 1–4 张，2.6 Ext 一次一张并支持 SVG／GIF。需要蒙版时选对应的标准模型。[2 Ext](https://docs.apimart.ai/cn/api-reference/images/gpt-image-2/generation.md) · [2 官方](https://docs.apimart.ai/cn/api-reference/images/gpt-image-2/official.md) · [2.5 Ext](https://docs.apimart.ai/cn/api-reference/images/gpt-image-2.5-ext/generation.md) · [2.6 Ext](https://docs.apimart.ai/cn/api-reference/images/gpt-image-2.6-ext/generation.md) |
| Grok Imagine 2.0 Image | Ext 文生最多 12 张，图生最多 5 张参考且输出一张。官方版最多 3 张参考，可设 1K／2K。Ext 合格单图提供原任务选区编辑。[Ext](https://docs.apimart.ai/cn/api-reference/images/grok-imagine-2.0-ext/generation.md) · [官方](https://docs.apimart.ai/cn/api-reference/images/grok-imagine-2.0-ext/official.md) |
| Grok Imagine 1.5 Video | Ext 支持首尾帧及最多 14 张参考图。参考图或首尾帧模式最高 720p。官方 1.5 接受最多 7 张参考图、最高 1080p。两版分别提交到各自模型 ID。[Ext](https://docs.apimart.ai/cn/api-reference/videos/grok-imagine/generation.md) · [官方](https://docs.apimart.ai/cn/api-reference/videos/grok-imagine/official.md) |
| Veo 3.1 | Fast／Quality 的 Ext 是兼容别名。Ext 固定 8 秒，符合条件时可续到 15 秒。官方 Fast／Quality 可选 4、6、8 秒，有人物策略和多样本参数。[Ext](https://docs.apimart.ai/cn/api-reference/videos/veo3/generation.md) · [官方](https://docs.apimart.ai/cn/api-reference/videos/veo3/generation-official.md) |
| Gemini Omni 1.1 Flash | Ext 接受 1 或 3 张参考图和最多 1 段参考视频，时长为 4、6、8、10 秒。官方版提供首尾帧、视频编辑和续写。[Ext](https://docs.apimart.ai/cn/api-reference/videos/omni-flash-ext/generation.md) · [官方](https://docs.apimart.ai/cn/api-reference/videos/gemini-omni-1.1-flash/generation.md) |
| Gemini Omni 1.2 Flash | 两版均有首尾帧、视频编辑和续写。Ext 增加系统指令、负面提示词，首尾帧除外。Ext 的素材编辑／续写接受 1 段视频加最多 10 张参考图。原任务按钮继续使用当前账号和原型号的回执。[Ext](https://docs.apimart.ai/cn/api-reference/videos/gemini-omni-1.2-flash-ext/generation.md) · [普通版](https://docs.apimart.ai/cn/api-reference/videos/gemini-omni-1.2-flash/generation.md) |
| Muse Image／Video | 插件提供 Ext。图像返回一张 WebP。视频返回 2–10 秒的 720p 有声 MP4。两者最多 9 张参考图。[图像](https://docs.apimart.ai/cn/api-reference/images/muse-image-ext/generation.md) · [视频](https://docs.apimart.ai/cn/api-reference/videos/muse-video-ext/generation.md) |
