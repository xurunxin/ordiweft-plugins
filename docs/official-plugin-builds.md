# 官方业务插件构建

这批源码从 Ordiweft `08fad08976b66036a846542d711c9822f88f4bb2` 导入。各包保留 `LICENSE`、`NOTICE`、版本、原 provider 和模型 ID。

## 开发与接入

1. 接入新供应商、模型或功能前，先查下方源码入口、相关插件的 manifest 和[官方 npm 版本清单](../repository/npm-releases.json)。已有实现优先扩展或复用。核对界面可用性时，再确认目标部署的宿主支持、已安装及活动版本、渠道和模型开关、凭据与权限。
2. 在本仓库维护插件代码、模型声明和发行配方。现有 SDK 或宿主运行时缺少接入能力时，在 [Ordiweft](https://github.com/xurunxin/Ordiweft) 补齐所需契约和共用能力，并用兼容宿主验证安装、配置与调用。
3. 接入变更随文档更新。本页记录源码入口、发行包和宿主依赖，插件 README 记录配置、模型能力及验证方式。官方插件按 [npm 发布与上架流程](../README.md#npm-发布与上架)发行，核对公开版本和制品摘要后更新官方清单，供部署从插件中心安装。

## 插件源码与部署依赖

| 插件 | 源码 | 包 ID | 部署依赖 |
| --- | --- | --- | --- |
| Codex 图片订阅 | [codex-image](../examples/plugin-go/codex-image) | `ordiweft-migrations/codex` | 宿主受信 Codex 登录 |
| GPT Image | [gpt-image](../examples/plugin-go/gpt-image) | `ordiweft-examples/gpt-image` | OpenAI API 连接 |
| MiniMax | [minimax](../examples/plugin-projects/minimax) | `ordiweft-migrations/minimax` | MiniMax 独立渠道服务 |
| 即梦／火山方舟 | [ark-jimeng](../examples/plugin-projects/ark-jimeng) | `ordiweft-migrations/volcengine` | 即梦／方舟独立渠道服务 |
| 豆包语音 | [doubao-speech](../examples/plugin-go/doubao-speech) | `ordiweft-migrations/doubao-speech` | 官方语音 API Key 与宿主原任务流 |
| Fish Audio | [fish-audio](../examples/plugin-go/fish-audio) | `ordiweft-migrations/fish-audio` | Fish Audio API Key、原音色目录和请求编码 |
| 百炼万相 2.7 | [dashscope-wan](../examples/plugin-go/dashscope-wan) | `ordiweft-examples/dashscope-wan` | 百炼区域 API 连接和受审 OSS 上传策略 |
| 百炼完整渠道 | [源码](../native/dashscope) · [发行配方](../plugins/ordiweft-migrations/dashscope/1.1.0/recipe.json) | `ordiweft-migrations/dashscope` | 支持 `dashscope-exec-v1` 的宿主、原工作空间与账号配置 |
| Higgsfield | [源码](../native/higgsfield) · [发行配方](../plugins/ordiweft-migrations/higgsfield/1.1.0/recipe.json) | `ordiweft-migrations/higgsfield` | 支持 `higgsfield-stdio-v1` 并记录该发行摘要的宿主、Higgsfield API 凭据 |
| APIMart | [源码与安装说明](../native/apimart/README.md) · [1.2.0 发行配方](../plugins/ordiweft-migrations/apimart/1.2.0/recipe.json) | `ordiweft-migrations/apimart` | 支持 `apimart-stdio-v1` 并记录该发行摘要的宿主、APIMart API Key、公开黄金价查询 |

## Go 适配器

完整百炼包 1.1.0 的静态 Linux amd64 执行器从 [外仓入口](../executors/dashscope) 构建，隔离环境使用随包固定的二进制。新任务核对活动版本，原任务按原发行摘要与票据查询。宿主继续保管账号凭据、地域计费与原音色复刻缓存。数据型 1.0.0 继续用于历史回退。

```powershell
pwsh.exe -NoProfile -File scripts/build-bailian-executor.ps1 -HostSource <受审宿主工作树> -Version 1.1.0 -Out <新仓外目录>
```

构建使用 Go 1.27，产出适配器、版本化模型清单和文件摘要。用可信宿主 CLI 打包并核对原 SHA-256，再按 `registry.mjs check`、`build`、`prepare` 收集候选审阅材料。万相 2.7 ZIP 保留为独立示例。

准备从上述宿主提交导出的公开 Go SDK，核对 `sdk-export.json`，安装与快照一致的 Go。分别运行下列命令，构建器创建新的仓外目录，编译 Linux amd64 静态适配器，记录 SDK、源码、构建工具和制品摘要。

```powershell
node scripts/build-plugin-go-example.mjs --example codex-image --sdk <公开SDK绝对路径> --sdk-commit 08fad08976b66036a846542d711c9822f88f4bb2 --out <新仓外目录>
node scripts/build-plugin-go-example.mjs --example gpt-image --sdk <公开SDK绝对路径> --sdk-commit 08fad08976b66036a846542d711c9822f88f4bb2 --out <另一个新仓外目录>
```

在各自的 `bundle` 目录可运行 `go test ./...`，然后用可信 `ordiweft-plugin pack -root <bundle> -out <新ZIP>` 生成候选包。正式晋级须从干净提交构建，并比较已有同版本制品摘要。制品内容变化须提升版本。

第二批纳入 IndexTTS、豆包 SeedAudio 和百炼万相。分别使用 `--example indextts`、`--example doubao-audio` 与 `--example dashscope-wan`。构建入口保存可重建候选和现有包标识。百炼候选覆盖 Wan2.7 图像和图像专业版，完整百炼渠道切换还需验证其他模型、音色和区域配额。渠道切换需在宿主上核对原生模型、原任务绑定、兼容授权与回退条件后执行。

豆包语音使用 `--example doubao-speech`，沿用原生渠道的官方 2.0 模型、参数和计量字段。宿主需提供 `connections.voice-request-fields` v1。候选验证、原生授权、活动版本切换与回退使用现有宿主接口。

豆包语音 `1.0.0` 的[固定配方](../plugins/ordiweft-migrations/doubao-speech/1.0.0/recipe.json)绑定外仓提交 `d926f40468b96a30c8aa22cb8858b2939726473e`、宿主 SDK 提交 `6ed4d8675eb8c071d126b3743d1acc3f7cddabcb` 和 Go 1.27.0。IndexTTS 的 [1.0.0 配方](../plugins/ordiweft-examples/indextts/1.0.0/recipe.json)继续复现原 ZIP。两个 npm 候选见[候选清单](../repository/npm-candidates.json)。

Fish Audio 使用 `--example fish-audio`。宿主需支持 `connections.fish-audio-tts`、`connections.repeated-form-fields` 及 1.1.0 安装夹具的音色和回执验证。合成、零样本合成、设计候选排序及私有克隆在隔离 Linux 中分别验证。豆包语音和 Fish Audio 构建需使用包含各自上述能力的宿主 SDK。

Fish Audio `1.1.0` 的[固定配方](../plugins/ordiweft-migrations/fish-audio/1.1.0/recipe.json)使用外仓提交 `d926f40468b96a30c8aa22cb8858b2939726473e`、宿主 SDK 提交 `c538cddb70fcf3a7f2a47c5cc7b9872d78ae345f` 和 Go 1.27.0。npm 包为 `@daoxialiuren/ordiweft-fish-audio`。安装包的音色搜索和详情通过宿主只读目录处理。

## 保留原配方

[社区包草稿 PR #1](https://github.com/xurunxin/ordiweft-plugins/pull/1) 固定 26 个候选版本的路径和历史来源。SeedAudio 1.0.3 与百炼万相 1.0.0 可以直接复用各自原 `portable-pack` 配方。发布既有版本时使用原配方及原制品摘要，本页的源码构建入口用于开发新版本。

- [SeedAudio 1.0.3](../plugins/ordiweft-examples/doubao-audio/1.0.3/recipe.json) 覆盖纯文本音频生成。
- [百炼万相 1.0.0](../plugins/ordiweft-examples/dashscope-wan/1.0.0/recipe.json) 的原 OSS 上传和图像任务票据可复用。完整百炼迁移还涉及 Qwen 图像、视频票据、语音合成、音色创建、工作空间与配额。
- Fish Audio 的新候选位于上表的源码目录，保留 MsgPack 合成请求、原音色绑定及宿主音色目录。

Higgsfield 以 `@daoxialiuren/ordiweft-higgsfield` 分发 Linux amd64 和 Windows amd64 预构建适配器。插件中心校验发行摘要并选取当前平台程序。部署侧只需在激活后配置渠道凭据。构建、权限边界和模型验收见 [Higgsfield 维护说明](../native/higgsfield/README.md)。

APIMart 使用 `@daoxialiuren/ordiweft-apimart` 分发 Linux amd64 与 Windows amd64 适配器。其发行清单固定两个程序和完整模型声明。宿主把上传回执保存在独立于发行目录的渠道状态目录。

1.3.0 的构建需使用包含 `aigc.ModelGenerationMode` 的 Go SDK 导出。宿主通过该声明合并模型菜单并判定自动模式，定价、提交与原任务恢复继续按具体模型 ID 处理。1.3.1 需使用含 `Model.AudioRequiresVisual` 与 `PromptSpec.ReferenceSyntax` 的宿主和 SDK，以便在画布与适配器执行相同的多模态约束。1.3.2 需要支持 `ModelGenerationMode.TextLabel` 的宿主和 Go SDK，才能用同一执行模型呈现文生和有素材的生成方式。1.3.3 使用支持背景音频模式、可省略的模式参数和可编辑续写提示词的宿主与 Go SDK。线上安装需先审查并登记正式制品摘要。

## Node 适配器

使用 Node 24.20.0，并按各项目锁文件准备原 SDK、私有维护工具归档及 TypeScript 缓存。归档由维护者单独取得。按包 README 在隔离构建目录执行 `npm ci --offline --ignore-scripts --no-audit --no-fund`、`npm test` 和 `npm run build`，再用可信 CLI 打包。MiniMax 和即梦渠道进程由部署者独立安装和管理。

## 推送到安装来源

用现有 `catalog-index` 生成候选目录，指定维护者审查通过的 ZIP 后，交给 TUF 维护者签发。公开入口 `repository/source.json` 使用宿主的来源定义格式，包括 `id`、`metadataUrl`、`targetsUrl`、`root`（初始公开根原字节的 Base64）、`rootSHA256`、`publishers`、`enabled`。GitHub 仓库主页通过 `main/repository/source.json` 发现配置。通过独立可信渠道提供初始根指纹。签名目录、ZIP 和旧版元数据按原发布策略保存。
