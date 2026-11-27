# APIMart for Ordiweft

APIMart 接入 Ordiweft 的图像、视频、音乐、配音和语音转录入口。参数由插件清单驱动画布原有控件。参考图、首尾帧和多模态素材通过现有画布输入提供。

## 安装

使用支持 1 MiB 进程插件清单和 `taskContinuation` 的 Ordiweft 宿主。将适用平台 ZIP 解压到宿主可信的 `data_dir/plugins/apimart`。Linux 上为 `provider` 赋予执行权限。核对 `plugin.json` 的程序 SHA-256 后重启宿主，再在管理后台的「渠道与模型」启用 APIMart。将插件的 `API_KEY` 映射到宿主服务环境中的 `APIMART_API_KEY`。

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

## 从原任务续作

- 选择成功的 MiniMax H3 768P 版本，在版本面板点击「升为 2K」创建再生成节点。支持普通生成及首尾帧，沿用上游原任务的内容和参考。[H3 Regeneration 文档](https://docs.apimart.ai/cn/api-reference/videos/minimax-h3/regeneration)
- 用 Seedance 2.5 的 480p 草稿或首尾帧草稿模式生成预览，选择版本并创建 1080p 正式片。任务自上游创建起 3 天内有效，正式片沿用原内容、时长、画幅和参考。[Seedance 2.5 文档](https://docs.apimart.ai/cn/api-reference/videos/seedance-2-5/generation)

创建续作节点只保存来源和设置。在新节点中点击生成才提交计费请求。宿主要求原任务归属当前用户、原版本仍在同一画布且凭据一致。早期插件回执缺少这项证明。黄金价报价使用原版本时长及已测量的参考视频，H3 计入超过 5 张的参考图，Seedance 按对应 Token 档位呈现估算。所需计量缺失时显示待核实，实扣保留 APIMart 回执。

## 构建与核验

从兼容宿主导出公开 Go SDK。在 Windows 上使用 Go 1.27 和 PowerShell 7 执行：

```powershell
pwsh.exe -NoProfile -File scripts/build-apimart.ps1 -Sdk <已导出的公开SDK> -Out <新仓外目录>
```

构建器在无外部 Go 模块下载的模式下测试适配器并生成 Windows amd64 和 Linux amd64 的 ZIP、程序文件和校验值。两个 ZIP 是受信进程部署包。插件中心的签名发行需要单独建包审阅。

用 Node 24 更新公开黄金价快照：

```powershell
node native/apimart/refresh-prices.mjs <仓外新价格JSON>
node native/apimart/build-catalog.mjs <上一步价格JSON> <核对日期YYYY-MM-DD> <新catalog.json>
```

对比差异并核实费率规则后再更新嵌入的 `catalog.json`。所有计费 POST 只发一次。异步受理后会先发 checkpoint，再查询原任务。对已接收的票据可在授权环境中运行 `provider status <model-id> <task-id>` 只读恢复，命令输出可能包含短期产物链接。

接口契约与各模型说明见 [APIMart 中文文档](https://docs.apimart.ai/cn)。
