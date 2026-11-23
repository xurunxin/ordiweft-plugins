# GPT Image 公开 SDK 代表候选

Apache-2.0。仅依赖固定公开Go SDK和标准库，没有HTTP客户端、API Key、业务数据库或Morpheus依赖。稳定provider为`gpt-image`，包为`ordiweft-examples/gpt-image`；1.1.5 保留 SVG 编写回执的计费字段，并保留 11 个模型定义，覆盖GPT Image 1与2系列的同步/SSE、PNG/JPEG/WebP及2.6 SVG/GIF/标注分支。它仍是待验收候选，未自动安装或替换现有部署；高容量组合与真实账号不能由小素材用例推定。

候选把无参考图请求编码为JSON生成，有参考图时使用原用途选择的multipart编辑；两种请求各有同步和SSE分支。JPEG背景沿原规则固定opaque，SSE固定单张；原模型字段、参考图用途与遮罩规则保持。GPT Image 2预设使用受审56项尺寸表，360构图追加原固定提示；自定义尺寸保留原值，隐藏预设不会覆盖。GIF帧时长按原10毫秒规则取整；SVG固定单张且无流。单底图标注使用image字段，多底图使用image[]，两者与mask保持独立。13份受审JSON映射固定原模型、提示词、参数、数量和输入分支；全部操作共用原generate效果槽，切换operation或callId不能重复提交。安全标识由原用户参数优先，否则由宿主原身份匿名哈希生成，原身份和凭据不进入插件。

`media.inspect`提供真实格式/像素和EXIF方向。遮罩仍校验透明PNG、原第一张图的展示尺寸与4MiB限制；标注校验透明PNG、同展示尺寸和底图16777216像素限制，不更改原图。候选输出由宿主检查真实格式，GIF还读取实际帧数并要求至少两帧，不能用请求帧数代替。程序只解析供应商JSON/SSE、报告原进度/用量，并通过公开StageOutputReaderAndReleaseBuffer分块返回解码字节；格式/尺寸、预算、持久化与采用归宿主。partial事件不保存为最终版本，只有原HTTP EOF、持久回执和唯一completed事件后才交回候选；断流、重复完成、结果数量不符或进程失败不重提。

`connection.profile.json`只是部署审查输入：官方固定origin为https://api.openai.com，API_KEY由宿主连接注入；任务至多一次远端调用。合成验证使用宿主独立夹具，不连接该origin、不消费真实Key、不启用渠道。管理员须另行绑定策略/连接并显式验证、激活；旧原生ID还须指定包授权。

当前请求体总上限64MiB、远端响应总上限256MiB，同步私有响应使用OpenAsset分块核对完整SHA与EOF，上限256MiB；每张候选解码不超过50MiB，任务仍受沙箱/帧数/时间及驻留总额度限制。它尚不是全部原生大素材组合的等价发行，完整独立消费、原生/公开参数和用途对照及真实供应商验收另行记录。模型、账号、音色与素材许可不随软件许可取得。

官方协议依据：[图片生成事件](https://developers.openai.com/api/reference/resources/images/generation-streaming-events)、[图片编辑事件](https://developers.openai.com/api/reference/resources/images/edit-streaming-events)、[编辑接口](https://developers.openai.com/api/reference/resources/images/methods/edit)。生成与编辑的完成事件分别为image_generation.completed和image_edit.completed，均从最终b64_json取图；模型是否对当前账号可用不能由此推定。

离线构建：先使用`scripts/export-plugin-go-sdk.ps1`导出固定干净SDK（至少af5d0aba的stage-buffer接口），再运行`node scripts/build-plugin-go-example.mjs --example gpt-image --sdk <SDK绝对路径> --sdk-commit <源码SHA> --out <全新仓外目录>`。工具链和源码/许可闭包进入build-proof，开发目录未提交时sourceDirty=true，不作为固定发行证明。构建不执行候选，也不发布。

1.1.2要求显式outputs.stage-buffer，旧严格宿主会在契约门禁拒绝该候选；原1.1.0及其在途任务保持原制品。JSON按单项读取，base64不再建立整份解码内存副本，每次只在沙箱临时目录暂存一张不超过50MiB的输出，完成后删除本任务自己的临时文件。SSE按行片段提供单事件读取，不累计整份响应；partial不是最终输出，完整EOF之后才提交唯一completed图。响应字段名仍按大小写不敏感读取，重复字段（包括大小写冲突）被拒绝，usage不超过64KiB，单JSON值不超过70MiB；模糊或超限结果保持未确认，不重提。Go软内存目标160MiB不替代容器硬限制。原输入句柄在原提交确认后释放，原HTTP请求/回执仍由宿主保存，不改变现有业务素材。

1.1.2显式申请diagnostics.report，失败仅报告固定本地分类或宿主已校验错误码；不报告JSON自由文本、上游内容、OS路径或秘密。该报告是插件来源证据，不确认退款、重试或业务结果；原stage进度仅记录输出序号，不改变原任务状态机。

1.1.3显式声明sandbox.scratchBytes=67108864，请求64MiB私有/tmp tmpfs以容纳逐张50MiB媒体；省略字段的旧包仍为32MiB。旧严格宿主在字段门禁拒绝新包。临时盘页仍计入256MiB硬内存上限；这不是额外输出额度，也不允许写宿主目录。

1.1.4对已选定b64_json字符串直接流式解码，不保留整份编码字符串；原始响应SHA/EOF仍由OpenAsset核验，周围JSON继续由标准jsontext验证。只转换当前图片字段，用量与其他元数据保留原值；非图片元数据/字符串最多64KiB。图片字符串要求ASCII base64，支持ASCII JSON转义和CR/LF；无效原始控制字符、非ASCII载荷或错误转义拒绝，即使该事件为partial。无效ASCII base64的partial仍只计进度。输入语法/来源错误不能被替换过程掩盖，损坏/重复/尾部错误不重提。
