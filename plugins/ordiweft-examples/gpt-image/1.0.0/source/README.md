# GPT Image 公开 SDK 代表候选

Apache-2.0。仅依赖固定公开Go SDK和标准库，没有HTTP客户端、API Key、业务数据库或Morpheus依赖。稳定provider为`gpt-image`，包为`ordiweft-examples/gpt-image`；当前只覆盖原`gpt-image-1.5`模型定义，**尚未完成整个原生渠道迁移**。其他原模型、GIF/SVG/标注能力、高容量组合和真实账号继续使用原实现，本候选不自动安装或替换现有部署。

候选把无参考图请求编码为JSON生成，有参考图时使用原用途选择的multipart编辑；两种请求各有同步和SSE分支。JPEG背景沿原规则固定opaque，SSE固定单张；原模型字段、参考图用途与遮罩规则保持。8份受审JSON映射固定原模型、提示词、参数、数量和输入分支；全部操作共用原generate效果槽，切换operation或callId不能重复提交。安全标识由原用户参数优先，否则由宿主原身份匿名哈希生成，原身份和凭据不进入插件。

`media.inspect`提供真实格式/像素和EXIF方向。遮罩仍校验透明PNG、原第一张图的展示尺寸与4MiB限制，不更改原图。程序只解析供应商JSON/SSE、报告原进度/用量，并通过公开StageOutput分块返回解码字节；格式/尺寸、预算、持久化与采用归宿主。partial事件不保存为最终版本，只有原HTTP EOF、持久回执和唯一completed事件后才交回候选；断流、重复完成、结果数量不符或进程失败不重提。

`connection.profile.json`只是部署审查输入：官方固定origin为https://api.openai.com，API_KEY由宿主连接注入；任务至多一次远端调用。合成验证使用宿主独立夹具，不连接该origin、不消费真实Key、不启用渠道。管理员须另行绑定策略/连接并显式验证、激活；旧原生ID还须指定包授权。

当前请求体总上限64MiB、远端响应总上限256MiB，同步私有响应读取上限64MiB；每张候选解码不超过50MiB，任务仍受沙箱/帧数/时间及驻留总额度限制。它不是全部原生大素材或所有模型的等价发行，完整独立消费、原生/公开参数和用途对照及真实供应商验收另行记录。模型、账号、音色与素材许可不随软件许可取得。

官方协议依据：[图片生成事件](https://developers.openai.com/api/reference/resources/images/generation-streaming-events)、[图片编辑事件](https://developers.openai.com/api/reference/resources/images/edit-streaming-events)、[编辑接口](https://developers.openai.com/api/reference/resources/images/methods/edit)。生成与编辑的完成事件分别为image_generation.completed和image_edit.completed，均从最终b64_json取图；模型是否对当前账号可用不能由此推定。

离线构建：先使用`scripts/export-plugin-go-sdk.ps1`导出固定干净SDK，再运行`node scripts/build-plugin-go-example.mjs --example gpt-image --sdk <SDK绝对路径> --sdk-commit <源码SHA> --out <全新仓外目录>`。工具链和源码/许可闭包进入build-proof，开发目录未提交时sourceDirty=true，不作为固定发行证明。构建不执行候选，也不发布。
