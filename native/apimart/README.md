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
- `audio-models.mjs` 描述 Flow Music、Lyria 3.5、Suno 音效和参考音频操作。Suno 音乐生成的灵感、自定义、歌词和纯音乐选项位于 `definitions.mjs`。Whisper-1 在转录入口提供时间段。GPT Audio Mini 默认关闭，兼容路由仍待该账户实测。
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
| APIMart Zoe | 文生视频、图生视频、多模态参考生视频、视频编辑 |
| Wan 3.0 / Prime | 文生视频、图生视频、多模态参考生视频 |
| Vidu Q4 Preview | 图生视频、参考生视频。[APIMart 文档](https://docs.apimart.ai/cn/api-reference/videos/vidu-q4-preview/generation) |
| FLUX 3 Video | 文生视频、图生视频（1–10 张按顺序的关键帧）、视频续写。[APIMart 文档](https://docs.apimart.ai/cn/api-reference/videos/flux-3-video/generation) |
| HappyHorse 1.0 | 文生视频、图生视频、参考图生视频、视频编辑。[APIMart 文档](https://docs.apimart.ai/cn/api-reference/videos/happyhorse-1.0/generation) |

对于 Seedance 2.5、Kling V4 和 Zoe，普通视频连线默认作为参考素材。视频编辑、视频延长和续写由用户显式选择，适配器发送各自的 APIMart 参数。Vidu Q4 的旧文生 ID 与 Wan 3.0 的旧编辑 ID 仅保留用于旧节点识别。上表列出当前插件接入的方式，Wan 3.0 的文件／链接参考和 Zoe 样片暂未接入。

## 从原任务续作

- 选择成功的 MiniMax H3 768P 版本，点击参数按钮旁的「升为 2K」创建续作节点。支持普通生成及首尾帧，沿用上游原任务的内容和参考。[H3 Regeneration 文档](https://docs.apimart.ai/cn/api-reference/videos/minimax-h3/regeneration)
- 用 Seedance 2.5 的 480p 草稿或首尾帧草稿模式生成预览，选择版本并创建 1080p 正式片。任务自上游创建起 3 天内有效，正式片沿用原内容、时长、画幅和参考。[Seedance 2.5 文档](https://docs.apimart.ai/cn/api-reference/videos/seedance-2-5/generation)

创建续作节点只保存来源和设置。在新节点中点击生成才提交计费请求。宿主要求原任务归属当前用户、原版本仍在同一画布且凭据一致。早期插件回执缺少这项证明。黄金价报价使用原版本时长及已测量的参考视频，H3 计入超过 5 张的参考图，Seedance 按对应 Token 档位呈现估算。所需计量缺失时显示待核实，实扣保留 APIMart 回执。

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
