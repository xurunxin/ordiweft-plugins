# APIMart for Ordiweft

APIMart 接入 Ordiweft 的图像、视频、音乐、配音和语音转录入口。参数由插件清单驱动画布原有控件。参考图、首尾帧和多模态素材通过现有画布输入提供。

从 1.3.0 起，支持 `generationMode` 清单字段的宿主在模型菜单合并同一上游模型的入口。参数面板按素材自动选择模式，也可手动固定首尾帧、编辑、续写或草稿。草稿和需要明确意图的音乐操作须手动选择。每种模式保留原模型 ID、参数及定价。

## 安装

官方 npm 包为 `@daoxialiuren/ordiweft-apimart`。使用支持 `apimart-stdio-v1` 且固定该发行摘要的 Ordiweft 宿主，在插件中心安装并激活。将插件的 `API_KEY` 映射到宿主服务环境中的 `APIMART_API_KEY`，然后在「渠道与模型」启用 APIMart。

本地开发可把对应平台的独立 ZIP 解压到 `data_dir/plugins/apimart`，在 Linux 为 `provider` 赋予执行权限，核对 `plugin.json` 的 SHA-256 后重启宿主。已有手动部署的实例应先处理原任务并退出独立进程部署，再从插件中心安装同渠道。

进程适配器以宿主服务身份运行，需要访问 APIMart 和产物存储。宿主分配 `data_dir/plugin-process-state/apimart` 保存上传状态。

## 参考素材

支持 Data URL 的模型可直接使用本地字节。要求公网链接的模型通过 APIMart 的图片、视频或音频上传接口准备本地文件，显式选择 3 天保存档。PNG、JPEG、GIF、WebP 默认单张上限 30 MB。MP4、MOV、WebM 默认单段上限 50 MB。MP3、WAV、M4A 默认单段上限 15 MB。另须满足模型参数面板中的引用数量以及模型本身的媒体规格。

画布参考还受宿主的单文件上传限制和进程协议 64 MiB 帧上限约束。帧包含引用文件的 Base64 与请求字段，合计原始字节须小于约 48 MiB。

插件按 API Key、当前用户与团队、媒体种类和文件 SHA-256 复用上传。并发任务共享持久上传记录，主机重启后仍使用原回执。有效期来自 APIMart 返回的 `expires_at`。插件给即将提交的任务保留 15 分钟使用余量。已过期的本地条目在下次使用时清理。公开上传文档提供上传与到期机制，已上传文件由 APIMart 按期回收。

图片、视频和音频共享账户的免费上传额度。插件记录最近一次已确认的配额与重置时间。若供应商拒绝新上传，插件在 `Retry-After` 或配额窗口释放前继续复用已有有效素材。额度耗尽后，已开启并确认的供应商付费上传设置可能产生费用。输出回执包含上传与复用计数、最早到期时间、已观察配额及实际上传收费；公网素材 URL 留在私有状态目录。

上传结果失联时保存待核实状态，避免对同一素材再次自动上传。已明确拒绝的上传在上游限制解除后，可由新任务重新发起。集群各实例的本地记录按各自状态目录维护，APIMart 的账户配额继续由其服务端执行。

[图片上传](https://docs.apimart.ai/cn/api-reference/uploads/images.md) · [视频上传](https://docs.apimart.ai/cn/api-reference/uploads/videos.md) · [音频上传](https://docs.apimart.ai/cn/api-reference/uploads/audios.md)

## 模型和报价

- `image-models.mjs` 描述参考图、蒙版、组图、精细度、画幅和分辨率。Nano banana 2.1 官方版支持 1–4 张，Ext 一次一张。
- `video-models.mjs` 描述各模型可选时长、分辨率、比例和引用。MAGI-2 分为文生、首尾帧、多模态参考。三种模式固定输出 1080p。
- `audio-models.mjs` 描述 Flow Music、Lyria 3.5、Suno 音效和参考音频操作。Suno 音乐生成的灵感、自定义、歌词和纯音乐选项位于 `definitions.mjs`。Whisper-1 在转录入口提供时间段。GPT Audio Mini 保留历史型号 ID，兼容路由仍待该账户实测。
- MiniMax H3 Context IR 输出结构化上下文。部分 Midjourney、Suno 和 Flow Music 编辑操作仍需接入各自的任务票据或专用产物。

插件对每个接入的模型声明公开定价来源和精确上游 ID。按张、按秒、按次和宿主可计量的费率进入 `pricing.rules`。需要更多用量字段的阶梯保持待核实。Nano banana 2.1 官方版的分辨率预扣与实际 Token 结算是两个字段。

宿主管理接口 `GET /api/admin/accounting/apimart-prices/{model}` 按插件模型读取当前黄金价。也可运行 `provider prices <model-id>`。费率取自 [APIMart 黄金价](https://apimart.ai/zh/pricing)与[公开价格 API](https://docs.apimart.ai/cn/api-reference/texts/qwen3.8-max/pricing)。价格刷新不消耗模型额度。任务实扣金额及积分取上游原回执。

## 视频多模态输入

| 模型 | 参考图 / 视频 / 音频上限 | 参考音频 | 提示词 |
| --- | --- | --- | --- |
| [MiniMax H3](https://docs.apimart.ai/cn/api-reference/videos/minimax-h3/generation) / [H3 Max](https://docs.apimart.ai/cn/api-reference/videos/minimax-h3/max) | 9 / 3 / 3 | 搭配图像或视频 | 7,000 字符 |
| [Seedance 2.0](https://docs.apimart.ai/cn/api-reference/videos/seedance-2-0/generation) | 9 / 3 / 3 | 搭配图像或视频 | 4,000 字符，Mini 按宿主请求长度上限 |
| [Seedance 2.5](https://docs.apimart.ai/cn/api-reference/videos/seedance-2-5/generation) | 30 / 10 / 10 | 可单独使用 | 支持 `@图片1`、`@视频1`、`@音频1` |
| [Seedance 2.5 Flash](https://docs.apimart.ai/cn/api-reference/videos/seedance-2-5-flash/generation) | 10 / 5 / 5 | 可单独使用 | 使用与 Seedance 2.5 相同的素材编号 |
| [MAGI-2](https://docs.apimart.ai/cn/api-reference/videos/magi-2/generation) | 10 / 5 / 5 | 可单独使用 | 15,000 字符 |

引用上限适用于多模态参考。H3 和 H3 Max 的首尾帧模式可单独指定首帧或尾帧；Seedance 和 MAGI-2 的首尾帧入口使用首帧及可选尾帧。Seedance 2.5 普通、Flash 和草稿的多模态入口会明确提交 `reference`，提示词仍须描述对应的生成意图。H3 Max 的 1080P 档会省略水印字段。

Wan 3.0 的多模态入口明确指定参考族，素材按「图1、视频1、音频1」编号。Kling V4 和 Flash 使用 `@Image1`、`@Video1`，标准版也可使用 `@Audio1`。APIMart Zoe 的音频参考需搭配图片或视频。Zoe 的长提示词仍受当前 Ordiweft 请求长度限制。[Wan 3.0](https://docs.apimart.ai/cn/api-reference/videos/wan3.0-video/generation) · [Kling V4](https://docs.apimart.ai/cn/api-reference/videos/kling-v4/generation) · [Kling V4 Flash](https://docs.apimart.ai/cn/api-reference/videos/kling-v4-flash/generation) · [APIMart Zoe](https://docs.apimart.ai/cn/api-reference/videos/apimart-zoe/generation)

## 生成方式分类

参数面板使用 APIMart 接口的模式名称与触发字段。同一个模型 ID 可以在面板显示文生和有素材的入口。自动模式按连线选择，手动固定文生或参考时会提示缺少或多余的素材，并保留现有连线。升级前使用同一个入口的画布节点会按已有连线显示文生或参考。

| APIMart 模型 | 参数面板的生成方式 |
| --- | --- |
| MiniMax H3 / H3 Max | 文生视频、图生视频（首帧、尾帧、首尾帧）、多模态参考生视频 |
| MAGI-2 | 文生视频、图生视频（首帧必填）、多模态参考生视频 |
| Seedance 2.0 标准 / Fast / Mini | 文生视频、图生视频、多模态参考生视频 |
| Seedance 2.5 / Flash | 文生视频、图生视频、多模态参考生视频、视频编辑、视频延长。标准版另外提供 480p 草稿入口 |
| Kling V4 / Flash | 文生视频、图生视频、参考生视频、视频编辑、视频续写。Flash 图生只接首帧，参考可接图和视频 |
| APIMart Zoe | 文生视频、图生视频、多模态参考生视频、视频编辑、480p 样片 |
| Wan 3.0 / Prime | 文生视频、图生视频、多模态参考生视频、文档或网页参考 |
| Vidu Q4 Preview | 图生视频、参考生视频。[APIMart 文档](https://docs.apimart.ai/cn/api-reference/videos/vidu-q4-preview/generation) |
| FLUX 3 Video | 文生视频、图生视频（1–10 张按顺序的关键帧）、视频续写、HD 草稿、续写草稿。[APIMart 文档](https://docs.apimart.ai/cn/api-reference/videos/flux-3-video/generation) |
| HappyHorse 1.0 | 文生视频、图生视频、参考图生视频、视频编辑。[APIMart 文档](https://docs.apimart.ai/cn/api-reference/videos/happyhorse-1.0/generation) |

对于 Seedance 2.5、Kling V4 和 Zoe，普通视频连线默认作为参考素材。视频编辑、视频延长和续写由用户显式选择，适配器发送各自的 APIMart 参数。Vidu Q4 的旧文生 ID 与 Wan 3.0 的旧编辑 ID 仅保留用于旧节点识别。Wan 3.0 的文档参考使用免登录的公网网址，在参数面板中选定「文档或网页参考」后可填写。

### 其他模型

| 模型 | 对应 APIMart 入口 |
| --- | --- |
| VEO 3.1 | Fast 的文生、帧转视频、参考图，Quality 的文生与帧转视频，Lite 的文生。适配器把旧画布保存的模式转换为接口接受的 `frame` / `reference`。[APIMart 文档](https://docs.apimart.ai/cn/api-reference/videos/veo3/generation) |
| Seedance 1.0 Pro 与海螺 | 分开呈现文生和图生。Seedance 1.0 Pro Fast 只接首帧，Quality 可接尾帧。海螺 2.3 只接首帧。[Seedance 1.0 Pro](https://docs.apimart.ai/cn/api-reference/videos/doubao/generation) · [海螺 2.3](https://docs.apimart.ai/cn/api-reference/videos/minimax-hailuo-2.3/generation) |
| SkyReels V4 | 文生、首尾帧及中间帧、多模态参考、拼贴图驱动、单主体声纹同步、视频扩展。适配器发送对应的带标签对象。[APIMart 文档](https://docs.apimart.ai/cn/api-reference/videos/skyreels-v4/generation) |
| Kling v3 Omni / Video O1 | 文生、首尾帧、参考图、源视频编辑与视频特征参考。视频输入走 `video_list`，素材引用按可灵格式编号。[v3 Omni](https://docs.apimart.ai/cn/api-reference/videos/kling-v3-omni/generation) · [Video O1](https://docs.apimart.ai/cn/api-reference/videos/kling-video-o1/generation) |
| Wan 2.5 / 2.6 / 2.7 | 区分图生和文生。文生可带背景音频，图生可用驱动音频。Wan 2.6 的图片特效使用一张参考图并选择模板。Wan 2.7 的视频续写使用独立入口。[Wan 2.5](https://docs.apimart.ai/cn/api-reference/videos/wan2.5/generation) · [Wan 2.6](https://docs.apimart.ai/cn/api-reference/videos/wan2.6/generation) · [Wan 2.7](https://docs.apimart.ai/cn/api-reference/videos/wan2.7/generation) |
| Vidu Q3 | Pro / Turbo 支持文生和图生。标准版及 Mix 使用参考生视频，要求参考图。[Pro / Turbo](https://docs.apimart.ai/cn/api-reference/videos/vidu-q3-pro/generation) · [标准版与 Mix](https://docs.apimart.ai/cn/api-reference/videos/vidu-q3/generation) |
| Grok Imagine 1.5 Video | 文生、参考图、图生（首帧或首尾帧）、首帧加参考图。同一执行家族提交到 APIMart 文档所用的 Ext 模型，保留旧画布 ID。[APIMart 文档](https://docs.apimart.ai/cn/api-reference/videos/grok-imagine/generation) |
| PixVerse V6 | 文生、单图、首尾帧过渡、多参考图融合，以及从已绑定任务续写。V6 Max 提供文生和单图。[V6](https://docs.apimart.ai/cn/api-reference/videos/pixverse-v6/generation) · [V6 Max](https://docs.apimart.ai/cn/api-reference/videos/pixverse-v6-max/generation) |
| Gemini Omni | 依档位提供文生、首帧或首尾帧、参考、视频编辑与续写。1.1 Flash 的编辑和续写使用 `metadata.task`，1.2 Flash 系列使用 `omni_reference_task_type`。[1.1 Flash](https://docs.apimart.ai/cn/api-reference/videos/gemini-omni-1.1-flash/generation) · [1.2 Flash](https://docs.apimart.ai/cn/api-reference/videos/gemini-omni-1.2-flash/generation) · [1.2 Flash Ext](https://docs.apimart.ai/cn/api-reference/videos/gemini-omni-1.2-flash-ext/generation) |

图像系列在同一执行模型中区分文生图与参考图生成。GPT Image 等模型的蒙版继续作为图像编辑的专用输入角色。可灵长图按 `skill` 分电商详情与网页设计，网页模式可指定风格图和主体图。Suno 的灵感、自定义、参考音频创作、上传翻唱、上传续写和音效沿用各自的字段或接口。[GPT Image 2.6](https://docs.apimart.ai/cn/api-reference/images/gpt-image-2.6/generation) · [可灵长图](https://docs.apimart.ai/cn/api-reference/images/kling-v4-long-image/generation) · [Suno](https://docs.apimart.ai/cn/api-reference/audios/suno/overview)

公开文档未列出模式契约的旧兼容型号保留原接入。SkyReels 同主体多图分组、Midjourney 和 Suno 的部分原任务编辑操作仍需单独接入。

## 从原任务续作

- 选择成功的 MiniMax H3 768P 版本，点击参数按钮旁的「升为 2K」创建续作节点。支持普通生成及首尾帧，沿用上游原任务的内容和参考。[H3 Regeneration 文档](https://docs.apimart.ai/cn/api-reference/videos/minimax-h3/regeneration)
- 用 Seedance 2.5 的 480p 草稿或首尾帧草稿模式生成预览，选择版本并点击「草稿转正片」。默认 1080p，原样片通道支持时可选 4K。3 天内可基于同一草稿创建正式片，沿用原内容、时长、画幅和参考。[Seedance 2.5 文档](https://docs.apimart.ai/cn/api-reference/videos/seedance-2-5/generation)

- APIMart Zoe 的 480p 样片有 24 小时的转正窗口，生成 1080p 正片时沿用原样片设置。[Zoe 文档](https://docs.apimart.ai/cn/api-reference/videos/apimart-zoe/generation)
- FLUX 3 Video 的 HD 草稿或续写草稿可按 HD 或 FHD 渲染正片。续写草稿使用续写价档。[FLUX 3 Video 文档](https://docs.apimart.ai/cn/api-reference/videos/flux-3-video/generation)

创建续作节点只保存来源和设置。在新节点中点击生成才提交计费请求。宿主要求原任务归属当前用户、原版本仍在同一画布且凭据一致。早期插件回执缺少这项证明。黄金价报价使用原版本时长及已测量的参考视频，H3 计入超过 5 张的参考图，Seedance 按对应 Token 档位呈现估算。所需计量缺失时显示待核实，实扣保留 APIMart 回执。

## 1.3.4 参数核对

| 系列 | 已接入的参数与限制 |
| --- | --- |
| 图像 | 文档声明的提交前审核、Ext 官方兜底、GPT Image 2.6 规划强度与 SVG 写作配置、标注层、Seedream 5.5 图层拆分与 OCR 开关、Wan 2.7 框选坐标与配色、Qwen Image 改写模式、Midjourney 结构化权重与参考图网址。 |
| 视频 | 自动时长、MP4 / MOV、尾帧、VEO GIF、Seedance 联网工具、可灵分镜及参考主体、视频原声处理、PixVerse 连续片段、Omni Ext 系统指令与负面提示词、Wan 3.0 文档／网页、Wan 2.6 图片特效。 |
| 音频 | Suno 灵感生成的自定义模型和歌词改写，翻唱的灵感／自定义选项，延伸的 Persona，Whisper 转录提示词及温度。 |

画布连线提供图像、音频、视频素材。可灵主体图及 Midjourney 的角色、风格、深度图参数支持公网图像网址。任务 ID 由原版本绑定，回调和异步结果格式由适配器管理。Whisper 使用带时间段的 verbose JSON。VEO GIF 在 720p 选项下显示。Seedream 图层的高级 OCR 元数据尚无专用编辑器。开启官方兜底时，实际费用以渠道回执为准。

当前公开文档未给 DALL·E 3、Imagen 4、带人脸渠道的 Seedance 2.0、Grok Imagine 1.0 的旧图像与视频渠道、GPT Audio Mini 提供对应请求契约，沿用现有声明。[APIMart 中文 API 参考](https://docs.apimart.ai/cn/api-reference)

## 1.3.5 独立模型契约

- Nano Banana 2 Lite 与 Lite Ext 固定在 1K，支持提交前审核。[Lite 模型文档](https://docs.apimart.ai/cn/api-reference/images/gemini-3.1-flash/generation-lite.md)
- VEO 3.1 官方版按文生、首尾帧提供参数，支持人物策略和样本数量。Ext 的 Fast／Quality 可按绑定的原任务续写至 15 秒。[官方版](https://docs.apimart.ai/cn/api-reference/videos/veo3/generation-official.md) · [续写](https://docs.apimart.ai/cn/api-reference/videos/veo3/remix.md)
- HappyHorse 1.1 分为文生、首帧和参考图模式，支持 720P 和 1080P。Wan 2.7 R2V 支持首帧与角色图共用及各角色参考音色，VideoEdit 使用音轨处理元数据。Wan 2.6 I2V 与 Flash 均可选 720p／1080p，Flash 可切换有声或无声。[HappyHorse](https://docs.apimart.ai/cn/api-reference/videos/happyhorse-1.1/generation.md) · [Wan R2V](https://docs.apimart.ai/cn/api-reference/videos/wan2.7-r2v/generation.md) · [Wan VideoEdit](https://docs.apimart.ai/cn/api-reference/videos/wan2.7-videoedit/generation.md) · [Wan 2.6 I2V](https://docs.apimart.ai/cn/api-reference/videos/wan2.6/i2v-flash-generation.md)
- 可灵两档动作控制支持水印与按人物朝向校验源视频时长。SkyReels V4 的中间帧可指定秒点。[可灵动作控制](https://docs.apimart.ai/cn/api-reference/videos/kling-v2-6/kling-v2-6-motion-control-generation.md) · [SkyReels](https://docs.apimart.ai/cn/api-reference/videos/skyreels-v4/generation.md)
- Grok Imagine Image 2.0 官方版使用版本化任务回执，文生图可选质量。两档官方视频可用多张参考图，Base 版另有视频编辑入口。旧 1.5 官方编辑 ID 保留识别，重新执行时提示切换模型。[Grok 图像](https://docs.apimart.ai/cn/api-reference/images/grok-imagine-2.0-ext/official.md) · [Grok 视频](https://docs.apimart.ai/cn/api-reference/videos/grok-imagine/official.md)

## 1.3.6 派生产物和原任务续作

- MiniMax H3 Context-IR 在同一模型内提供纯文本、首尾帧和多模态参考，输出增强提示词并保存输入／输出 token 计量。可以复制结果或提取为画布文本节点。[Context-IR](https://docs.apimart.ai/cn/api-reference/videos/minimax-h3/context-ir.md)
- Seedream 5.5 按原图片 URL 的位置保存图层名称、边界框、OCR 行和文字样式。图层选项内开启富文本 OCR，结果可复制或提取为普通文本节点。旧草稿中的 OCR 开关在请求时转换为嵌套字段。[Seedream 5.5](https://docs.apimart.ai/cn/api-reference/images/seedream-5-5/generation.md)
- Suno 原音轨支持续写、翻唱、片段替换、单轨和全部分轨。Flow Music 原音轨支持续写、翻唱、片段替换和分轨。分轨 ZIP 由宿主展开，独立播放可校验的 WAV、MP3 和 FLAC 音轨。各操作使用已完成版本对应的服务端回执。同一任务返回多首歌曲时，Suno 按回执中的原始顺序选择。[Suno](https://docs.apimart.ai/cn/api-reference/audios/suno/overview.md) · [Flow Music](https://docs.apimart.ai/cn/api-reference/audios/flow-music/extend.md)
- Midjourney 原四宫格支持取单图、生成变体和重生成。Midjourney Video 支持首帧和首尾帧，480p／720p 与 1、2、4 条输出。黄金价按操作、档位及输出数量读取。[Midjourney](https://docs.apimart.ai/cn/api-reference/images/midjourney/upscale.md) · [Midjourney Video](https://docs.apimart.ai/cn/api-reference/images/midjourney/video.md)

- Grok Imagine 2.0 扩展版的已完成单图任务可创建局部编辑节点。确认后按原任务定位图像，校验选区后提交一次付费矩形编辑。[Grok 局部编辑](https://docs.apimart.ai/cn/api-reference/images/grok-imagine-2.0-ext/layer-region-edit.md)

## 1.3.7 高清放大与遮罩重绘

- Midjourney 从原四宫格的任务回执查找选定格的高清按钮。支持官方文档中的 V5 2×／4×、V6/V7 细节保持与创意增强，以源任务实际提供的按钮为准。[高清放大](https://docs.apimart.ai/cn/api-reference/images/midjourney/upscale.md)
- 从已保存的放大单图创建遮罩重绘节点。画布使用与 GPT Image 相同的遮罩编辑器，在底图像素坐标内绘制或导入透明 PNG。保存后才可执行。适配器先打开 inpaint 任务，收到 MODAL 状态后在同一任务提交一次遮罩，提示词可留空。两阶段之间保留原任务回执。[Inpaint](https://docs.apimart.ai/cn/api-reference/images/midjourney/inpaint.md) · [Modal](https://docs.apimart.ai/cn/api-reference/images/midjourney/modal.md)
- Grok 延用归一化矩形，增加精确多边形（含内部孔洞）与对象索引输入。对象索引可填写 API 分段结果中的原始索引，由插件核对原图分段结果；目前使用参数面板填写坐标和索引。[局部编辑](https://docs.apimart.ai/cn/api-reference/images/grok-imagine-2.0-ext/layer-region-edit.md)
GPT-4o Mini TTS 原始 PCM 尚缺 APIMart 的采样率、声道和位深契约。VEO 多样本预估与续写费用以实际任务回执或专用报价为准。

## 1.3.8 模型目录

隐藏 GPT-4o Mini TTS，以及 DALL·E 3、Imagen 4、Seedance 2.0 Face／Fast Face、Grok Imagine 1.0 文生图／图像编辑／视频、GPT Audio Mini。Grok Imagine 1.0 视频的文生、参考图与首尾帧执行条目一并隐藏。历史工程和模型 ID 保留。APIMart 默认配音节点从新建菜单移除。

## 构建与核验

从兼容宿主导出公开 Go SDK。在 Windows 上使用 Go 1.27 和 PowerShell 7 执行：

```powershell
pwsh.exe -NoProfile -File scripts/build-apimart.ps1 -Sdk <已导出的公开SDK> -Out <新仓外目录>
```

构建器在无外部 Go 模块下载的模式下测试适配器并生成 Windows amd64 和 Linux amd64 的独立 ZIP。官方发行使用干净提交、固定 SDK 和相同 Go 版本，追加 `-Release` 生成包含双平台程序、许可和构建凭据的 `source` 目录。用可信 `ordiweft-plugin pack` 打包，再按仓库的 npm 暂存发布流程审查。

用 Node 24 更新公开黄金价快照：

```powershell
node native/apimart/refresh-prices.mjs <仓外新价格JSON>
node native/apimart/build-catalog.mjs <上一步价格JSON> <核对日期YYYY-MM-DD> <新catalog.json>
```

对比差异并核实费率规则后再更新嵌入的 `catalog.json`。所有计费 POST 只发一次。异步受理后会先发 checkpoint，再查询原任务。对已接收的票据可在授权环境中运行 `provider status <model-id> <task-id>` 只读恢复，命令输出可能包含短期产物链接。

接口契约与各模型说明见 [APIMart 中文文档](https://docs.apimart.ai/cn)。

## 1.3.9 续写节点

生成结果旁提供两种快捷入口。需要原任务时，宿主从所选版本核对账户、回执和有效期后创建对应节点。需要源视频或音频时，宿主固定所选版本的素材并连到新节点，设置对应模式并打开参数和提示词。创建节点本身只保存画布。

| 续写方式 | 模型与参数 |
| --- | --- |
| 原任务续写 | VEO 3.1 Ext、PixVerse V6、Gemini Omni 的任务续写，以及 Suno／Flow Music 原音轨续作 |
| 视频素材续写 | Seedance 2.5／Flash、Kling V4／Flash、SkyReels V4 Fast／Standard、Wan 2.7、FLUX 3 Video、Gemini Omni 1.1 Flash／1.2 Flash／1.2 Flash Ext 使用各自的续写模式 |
| 音频素材续写 | Suno 使用 uploadExtend，固定音频作为 source_audio |
| H3 视频参考续写 | MiniMax-H3／H3-Max 使用已有多模态参考输入和可编辑的续写提示词。标准 H3 的合格 768P 原任务可升为 2K。 |

## 1.3.10 渠道版本和续作来源

模型菜单和参数面板会标明 Ext 与官方版的输入、输出和计费差异。Veo 3.1 Fast／Quality 仅在原回执确认关闭过官方兜底时提供原任务续写。Gemini Omni 1.2 Flash Ext 以源视频素材编辑和续写时可接入参考图。Grok Imagine 2.0 Image Ext 的分辨率跟随上游结果，历史像素档位参数在提交时省略。[模型能力、版本和原任务核对](../../docs/media-capabilities.md)

Seedance 2.5、Zoe 和 FLUX 3 Video 草稿转正，以及原任务的有效期和计费仍以对应接口为准。支持原任务和素材续写的模型可分别选择两个入口。
