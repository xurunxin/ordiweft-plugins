# 官方业务插件构建

这批源码从 Ordiweft `08fad08976b66036a846542d711c9822f88f4bb2` 导入。各包保留 `LICENSE`、`NOTICE`、版本、原 provider 和模型 ID。

| 插件 | 源码 | 包 ID | 部署依赖 |
| --- | --- | --- | --- |
| Codex 图片订阅 | [codex-image](../examples/plugin-go/codex-image) | `ordiweft-migrations/codex` | 宿主受信 Codex 登录 |
| GPT Image | [gpt-image](../examples/plugin-go/gpt-image) | `ordiweft-examples/gpt-image` | OpenAI API 连接 |
| MiniMax | [minimax](../examples/plugin-projects/minimax) | `ordiweft-migrations/minimax` | MiniMax 独立渠道服务 |
| 即梦／火山方舟 | [ark-jimeng](../examples/plugin-projects/ark-jimeng) | `ordiweft-migrations/volcengine` | 即梦／方舟独立渠道服务 |
| 豆包语音 | [doubao-speech](../examples/plugin-go/doubao-speech) | `ordiweft-migrations/doubao-speech` | 官方语音 API Key 与宿主原任务流 |
| Fish Audio | [fish-audio](../examples/plugin-go/fish-audio) | `ordiweft-migrations/fish-audio` | Fish Audio API Key、原音色目录和请求编码 |
| 百炼万相 2.7 | [dashscope-wan](../examples/plugin-go/dashscope-wan) | `ordiweft-examples/dashscope-wan` | 百炼区域 API 连接和受审 OSS 上传策略 |
| 百炼完整渠道 | [源码](../native/dashscope) · [发行配方](../plugins/ordiweft-migrations/dashscope/1.0.0/recipe.json) | `ordiweft-migrations/dashscope` | 支持 `dashscope-v1` 的宿主、原工作空间与账号配置 |

## Go 适配器

完整百炼使用独立维护的 Go 模块。宿主固定模块源码的提交与树摘要，用于提供 `dashscope-v1` 受信运行接口。渠道包可从签名目录安装后单独启用、升级和回退。包描述完整模型和节点，宿主提供按部署地域计算的定价、限流、工作空间、音色缓存及原票据查询。首次安装时保留原生回退记录。兼容的包版本可以更新模型展示名称和使用说明。新增模型协议、修改执行逻辑或计价规则需要更新宿主提供的运行接口。

新包使用可信 `ordiweft-plugin bailian-manifest -version <版本>` 导出可分发模型声明，生成新的数据包版本。按仓库现有 `registry.mjs check`、`build`、`prepare` 流程核对并打包。万相 2.7 ZIP 保留为单独示例。

准备从上述宿主提交导出的公开 Go SDK，核对 `sdk-export.json`，安装与快照一致的 Go。分别运行下列命令，构建器创建新的仓外目录，编译 Linux amd64 静态适配器，记录 SDK、源码、构建工具和制品摘要。

```powershell
node scripts/build-plugin-go-example.mjs --example codex-image --sdk <公开SDK绝对路径> --sdk-commit 08fad08976b66036a846542d711c9822f88f4bb2 --out <新仓外目录>
node scripts/build-plugin-go-example.mjs --example gpt-image --sdk <公开SDK绝对路径> --sdk-commit 08fad08976b66036a846542d711c9822f88f4bb2 --out <另一个新仓外目录>
```

在各自的 `bundle` 目录可运行 `go test ./...`，然后用可信 `ordiweft-plugin pack -root <bundle> -out <新ZIP>` 生成候选包。正式晋级须从干净提交构建，并比较已有同版本制品摘要。制品内容变化须提升版本。

第二批纳入 IndexTTS、豆包 SeedAudio 和百炼万相。分别使用 `--example indextts`、`--example doubao-audio` 与 `--example dashscope-wan`。构建入口保存可重建候选和现有包标识。百炼候选覆盖 Wan2.7 图像和图像专业版，完整百炼渠道切换还需验证其他模型、音色和区域配额。渠道切换需在宿主上核对原生模型、原任务绑定、兼容授权与回退条件后执行。

豆包语音使用 `--example doubao-speech`，沿用原生渠道的官方 2.0 模型、参数和计量字段。宿主需提供 `connections.voice-request-fields` v1。候选验证、原生授权、活动版本切换与回退使用现有宿主接口。

Fish Audio 使用 `--example fish-audio`。宿主需支持 `connections.fish-audio-tts`、`connections.repeated-form-fields` 及 1.1.0 安装夹具的音色和回执验证。合成、零样本合成、设计候选排序及私有克隆在隔离 Linux 中分别验证。豆包语音和 Fish Audio 构建需使用包含各自上述能力的宿主 SDK。

## 保留原配方

[社区包草稿 PR #1](https://github.com/xurunxin/ordiweft-plugins/pull/1) 固定 26 个候选版本的路径和历史来源。SeedAudio 1.0.3 与百炼万相 1.0.0 可以直接复用各自原 `portable-pack` 配方。发布既有版本时使用原配方及原制品摘要，本页的源码构建入口用于开发新版本。

- [SeedAudio 1.0.3](../plugins/ordiweft-examples/doubao-audio/1.0.3/recipe.json) 覆盖纯文本音频生成。
- [百炼万相 1.0.0](../plugins/ordiweft-examples/dashscope-wan/1.0.0/recipe.json) 的原 OSS 上传和图像任务票据可复用。完整百炼迁移还涉及 Qwen 图像、视频票据、语音合成、音色创建、工作空间与配额。
- Fish Audio 的新候选位于上表的源码目录，保留 MsgPack 合成请求、原音色绑定及宿主音色目录。

## Node 适配器

使用 Node 24.20.0，并按各项目锁文件准备原 SDK、私有维护工具归档及 TypeScript 缓存。归档由维护者单独取得。按包 README 在隔离构建目录执行 `npm ci --offline --ignore-scripts --no-audit --no-fund`、`npm test` 和 `npm run build`，再用可信 CLI 打包。MiniMax 和即梦渠道进程由部署者独立安装和管理。

## 推送到安装来源

用现有 `catalog-index` 生成候选目录，指定维护者审查通过的 ZIP 后，交给 TUF 维护者签发。公开入口 `repository/source.json` 使用宿主的来源定义格式，包括 `id`、`metadataUrl`、`targetsUrl`、`root`（初始公开根原字节的 Base64）、`rootSHA256`、`publishers`、`enabled`。GitHub 仓库主页通过 `main/repository/source.json` 发现配置。通过独立可信渠道提供初始根指纹。签名目录、ZIP 和旧版元数据按原发布策略保存。
