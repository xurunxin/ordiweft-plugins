# MiniMax 独立接入包

接入包 2.2.5，target 为 minimax。安装、native 迁移预检/授权、连接绑定、合成验证及显式激活均沿用插件平台；保留原生实现与旧任务，不自行发布或切换生产。旧型号缺省只走官方 API，作为隐藏兼容型号保留，已有选中节点仍显示。新建菜单每个模型只保留一个普通名称，在同一模型的接入参数中选择 auto／订阅／官方，不重复列出兼容型号；默认优先使用管理员共享的 MiniMax Design 账户。

标准 MiniMax-H3 的「全能参考」在 Design 订阅和官方 API 均支持图片、视频和音频。参考视频最多 3 段，每段 2–15 秒，总长最多 15 秒，单文件最多 50 MB，混合素材最多 12 个。Design 支持最多 9 张图片，视频和音频合计最多 3 段。宿主核对 MP4 原件，并按素材顺序向 Design 提供原件 URL 和视频轨道时长，用于订阅积分估算与提交。接入包要求渠道 0.4.0、`channel.design-video-reference@1` 和宿主 `connections.video-reference-duration@1`。[MiniMax H3 全能参考](https://design.minimax.io/h3)

2.2.0 新增隐藏的 `speech.transcribe` 模型 `asr-1.0`，供系统语音输入和剪辑转写共同使用，不提供普通生成节点。要求宿主 `generation.transcription@1` 和渠道0.3.0的 `channel.transcription@1`；不兼容的旧宿主必须在激活／执行前拒绝。固定源音频的实际字节与时长先由宿主核对，转写结果通过结构化候选返回句段、字词与说话人毫秒时间，不冒充声学强制对齐。识别费用和原回执保留，失败不自动重新发送。SRT 交换与字幕采用由宿主执行，插件不能直接改剪辑文档。

2.1.4 补齐 H3 系列时长字段的 duration 语义，供 Agent 推荐与任务交付要求核对，不再把明确传入的时长误判为未声明；API 请求与收费规则沿用原实现。

配套渠道服务见 [MiniMax channel](../../../tools/minimax-channel/README.md)。API Key 和订阅令牌只由该服务消费，包本身仅通过宿主 connections 调用管理员绑定的 CHANNEL_URL / CHANNEL_TOKEN。API_KEY 仅保留历史设置兼容。连接策略在 fixtures；JSON 策略摘要由固定公开 Go SDK 的 JSONRequestPolicy.Digest 计算，装配后才可绑定，模板占位不是可用配置。

构建使用固定维护工具 0.2.3、公开 Node SDK 0.1.15 与 TypeScript 5.9.3。审阅归档放入 vendor 后，以禁用 hooks 的 npm ci 安装，执行 npm test、npm run typecheck、npm run build；不读取宿主源码树，也不需要桌面 Electron、原 MCP 或 Morpheus。vendor、node_modules、dist、bundle 不提交。新包要求 generation.quote、generation.voice-bindings 和 outputs.voice-identity 能力；不支持的旧宿主必须在提交前拒绝。

语音、音乐、音色创建保持独立。音色 ID 随 provider、compatibleModels 以及可选 channel/account 保存，仅能用于对应语音模型。音色试听有真实音频时，可从 audio-out 接到明确支持音频引用的其他模型；不会把音色 ID 当作音频。2.1.3 为标准 H3 / H3-auto 增加 `reference` 图片与音频参考模式，最多9张图片、3段音频；WAV/MP3 单段2–15秒、合计不超过15秒、每段最多15MB。宿主校验原字节事实，并通过已绑定策略提供供应商可读的音频 URL；需配置私有 CDN。图片参考与首尾帧模式保持原有语义，其他型号不因此获得音频能力。

2.1.3 要求渠道0.2.2及 `channel.audio-reference` 能力。在尚未适配音频参考的订阅路径上，可通过模型的「接入方式」显式选择兼容的官方 API。只读 quote 传递参考数量，不传原媒体字节；标准 H3 前5张参考图和音频不额外收费，第6张起每张0.20元，其他缺少计量的参考模式保持未知。费率来源为[官方价目表](https://platform.minimax.cn/docs/pricing/overview)，报价不能代替供应商最终账单。

订阅参数按可表达的子集提供；精确采样率、声道、格式等官方设置只在官方接入方式下显示。自动模式仅在确认 Design 积分不足时使用兼容的官方 API。模型或参数不兼容时保留准确的 Design 失败原因，支持相关能力的模型可显式选择官方 API。订阅私有音色 ID 仍按原账户校验。本地真实验收与限制见[记录](../../../docs/evidence/minimax-live-validation-20261115.md)；长期续期与生产部署未验收。

所有生成默认关闭水印，并向上游显式发送 `aigc_watermark:false`。标准 H3 的 768P 可标记为再生成样片；成功后通过现有样片操作创建 `MiniMax-H3-Regeneration` 节点，固定原任务及内容，官方再生成输出 2K。不能把任意视频当作该接口的原样片。

安装包本地调度为单并发，与当前同一隔离执行实例的单槽一致；这不是 MiniMax 远程配额。其他请求在宿主队列等待，渠道仍查询订阅实际模型并发。

2.1.0 配套渠道 0.2.0，新增 General Image 2 / Pro 的显示名 Nano Bnana 2 / Nano Bnana Pro、Midjourney V8.2 与 ElevenLabs Music v2。后三方模型仅 Design 订阅提供，不映射为 MiniMax 官方型号；MJ 固定一次四张，当前目录支持至多四张参考图。Music 3.0 与 ElevenLabs 使用同一音乐节点，各自字段由 schema 驱动，后者当前仅支持纯音乐。

quote 连接为只读 `/v1/quotes`；fixtures/policies/quote.json 与生成策略分别绑定。Plus 包年配置与参考换算、能力来源和本地验证范围见[本轮证据](../../../docs/evidence/unified-model-design-20261115.md)。没有 quote 绑定的旧包继续执行原任务，只不显示新报价。
