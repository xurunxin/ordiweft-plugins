# Codex 订阅独立插件

`ordiweft-migrations/codex` 2.0.0 沿用 `codex` provider，通过公开 Go SDK 和 Host Services 执行。适配器不持有网络、登录目录或 Token；管理员显式授权原生迁移、安装、绑定两份 JSON 策略和 `credentialProvider:codex-profile` 连接、合成验证并激活后，新统一图像节点使用 SDK 协议。旧节点只读并显示“已弃用”。不自动修改生产或注销旧任务的原 handler。

无参考图只请求 generations，有图只请求 edits；原请求共用一个 generate 效果槽。登录刷新沿用宿主管理的 Codex 登录，原账户/目录指纹改变时拒绝，不用 API Key 替代或切换端点重发。比例和风格由提示词表达；仅背景进入支持参数，size/quality 固定 auto、单张请求，不把订阅端的模型/尺寸回显当作能力保证。

模型字段来源于原 Codex manifest，隐藏历史型号保留。响应沿用公开 GPT Image 候选的有界流式 base64 解码，读取完整 SHA/EOF 后完成；真实媒体格式、像素和采用由宿主校验。请求总上限 64 MiB，响应 256 MiB，单图 50 MiB；私有 tmpfs 64 MiB。异常保留原回执且不自动重试。

使用 `scripts/export-plugin-go-sdk.ps1` 导出当前 SDK 后，通过 `scripts/build-plugin-go-example.mjs --example codex-image --sdk <SDK目录> --sdk-commit <源码SHA> --out <全新仓外目录>` 离线构建。开发快照明确标记 sourceDirty，不冒充固定发行。连接 profile 的策略摘要须先用 SDK Digest 计算后填写，不能手工猜测。验收夹具是本地合成 PNG，无真实订阅调用。

构建器从同仓的 `gpt-image/payload.go` 取流式解码源码并纳入来源检查。导出的包内自带 `payload.go` 与完整 `sdk/`，可离开本仓单独构建。
