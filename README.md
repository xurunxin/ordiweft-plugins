# Ordiweft 插件源码仓库

保存可审阅源码、固定配方与发布提案。Ordiweft 支持从官方 npm 清单、TUF 签名目录或离线 ZIP 安装插件。已批准用于官方 npm 安装的版本以 [发布清单](repository/npm-releases.json) 为准。

原创工具与模板采用 [Apache-2.0](LICENSE)，保留 [NOTICE](NOTICE)；每个包携带自己的许可与来源。Morpheus 保持其现有许可，本仓库不分发 Morpheus 或其依赖。

## 新旧模式共存

官方业务插件的维护源码见[官方插件构建](docs/official-plugin-builds.md)。保留的社区配方可继续独立检查和打包。

首批官方源码覆盖 Codex、GPT Image、MiniMax 和即梦／火山方舟。原生渠道逐个验证功能对等后迁移。SDK v1/v2 受信进程、ComfyUI 及专用服务仍可与独立插件共存。新插件使用独立 provider ID 和包命名空间。接管已有 ID 使用宿主的兼容授权，同 ID 安装、更新和回退遵循原有宿主门禁。

凭据、权限、原 HTTP 回执、媒体校验、版本绑定及业务写入沿用宿主 Host Services。官方插件从固定依赖的 Go 或 TypeScript 源码构建，现有社区配方继续打包经过审阅的预构建字节。普通安装与运行消费已发布的 ZIP。

## 当前可用范围

仓库保存26个不可变版本，覆盖内容/提示词模板/显式Skill、声明式表单与动作节点、文档/素材处理器、片场素材与环境预设、外部worker声明，以及IndexTTS、百炼Wan图片、GPT Image和SeedAudio代表候选。每个版本携带固定来源、配方、完整文件清单与LICENSE/NOTICE；静态Go包另保存公开SDK、Go许可、预构建入口和原构建证明。它们保留原package/provider/model ID与旧版本，不自动替换完整渠道或已有节点/任务。具体能力和验收范围以各固定记录为准。

GPT Image 1.1.4逐张流式解码当前图片字段，显式请求64MiB临时盘并移交输出buffer。它需要支持sandbox.scratchBytes和outputs.stage-buffer的宿主，旧严格工具拒绝新字段。原25个版本的源码/配方和制品保持；整个渠道的大输入组合、真实账号、完整平台与正式发行并未由这些代表路径自动完成。

`portable-pack` 仅校验和打包预构建字节，不执行插件、npm 脚本、依赖安装或模型请求。它不是通用 TypeScript/npm 编译沙箱。Node与静态process包所需的隔离运行镜像另固定摘要，安装时仍由部署管理员决定是否具备相应运行器。worker配方不能选择运行镜像或端点，服务部署和隔离另行管理。

```text
publishers/<publisher>/owner.json
plugins/<publisher>/<name>/<version>/recipe.json
plugins/<publisher>/<name>/<version>/source/
repository.json
scripts/registry.mjs
```

## 本地检查与打包

需要 Node.js ≥22.19，无 npm 依赖。Ordiweft CLI 应从 repository.json 固定提交构建，由维护者通过绝对路径和 SHA-256 显式提供。工具不会下载或替换 CLI。

```powershell
node scripts/registry.mjs check
node --test tests/registry.test.mjs

# 在固定提交的 Ordiweft checkout 中构建可信打包工具。
go build -trimpath -o <仓外CLI路径> ./cmd/ordiweft-plugin

# 回到本仓库；输出目录必须为仓外尚不存在的目录。
node scripts/registry.mjs build <CLI绝对路径> <CLI的SHA256> <仓外新目录>
```

输出 ZIP 与 build-proof.json。v3证明分别记录实际制品SHA/字节数、宿主release digest、选择的CLI摘要/配置的工具源码提交，以及逐包原源码位置/提交和配方JSON序列化摘要recipeDigest；JSON.parse后JSON.stringify的UTF-8字节参与摘要，避免CRLF/LF差异。新工具可以打包旧原源码，不把旧源码错误标为工具提交。prepare重新核对原配方来源和ZIP；v2仍按原配方字节SHA读取，不改写历史证明。retainedHostSources用于读取历史证明；缺逐包来源的v1证明不能声称不同源码提交，其提案仅保留原共享工具提交的来源范围，不补造原路径或配方指纹。

这些是待复核的构建记录，不是签名、命名空间授权或构建证明服务；配置的工具提交不能单凭文件自称得到认证，维护者仍须独立确认所提供CLI摘要与真实构建来源。所有输出不覆盖已有文件，失败保留现场供检查。文件完整性以真实宿主CLI为准，不在这里复制另一份release摘要算法。

## npm 发布与上架

百炼使用 `@daoxialiuren/ordiweft-dashscope` 分发 `ordiweft-migrations/dashscope`。SeedAudio 的待发布候选使用 `@daoxialiuren/ordiweft-doubao-audio` 分发 `ordiweft-examples/doubao-audio`，沿用 provider `doubao-audio`。包只包含原 ZIP、npm 元数据与许可文件。可提交暂存的精确版本和摘要见 [候选清单](repository/npm-candidates.json)。

首次发布由维护者使用对 `@daoxialiuren` 有相应权限的 npm 凭据提交暂存，再由维护者完成确认。后续在 npm 包设置中绑定 Trusted Publisher，使用 GitHub 仓库所有者 `xurunxin`、仓库 `ordiweft-plugins`、工作流 `npm-release.yml`、环境 `npm-release`，授予 stage 权限。GitHub 环境限定 `main`，在 Actions 页面手动选择包 ID 和审核过的版本。工作流复现原 ZIP 并交给 npm 暂存，维护者核对后批准公开发布。[npm Trusted Publishing](https://docs.npmjs.com/trusted-publishers/) · [npm 暂存发布](https://docs.npmjs.com/staged-publishing/)

```powershell
node scripts/npm-release.mjs ordiweft-examples/doubao-audio 1.0.3 <原始ZIP> <仓外新包目录>
npm pack <仓外新包目录> --ignore-scripts --pack-destination <仓外归档目录>
npm stage publish <生成的tgz> --ignore-scripts --access public
```

公开发布后，从 registry 下载该精确版本，核对内部 ZIP 摘要，再在单独 PR 中把相应候选条目加入正式清单并增加 `revision`。仅在保护的 `main` 合并审核通过的目录。撤回时保留条目并设置 `withdrawn:true`，再增加修订。已上架版本的摘要、包名和长度保持原值。TUF 发布继续使用下述签名流程。

## 候选与撤回提案

维护者先独立复核源码/许可、固定构建输入、实际权限、宿主兼容性及负向效果/恢复证明，再为原 build-proof.json 准备 selection.json：

```json
{
  "version": 1,
  "expectedBuildSHA256": "原build-proof.json的真实SHA256",
  "decisions": [
    {"key": "ordiweft-examples/typed-content/1.0.0", "state": "candidate", "reason": "提交候选审阅"}
  ]
}
```

```powershell
node scripts/registry.mjs prepare <build-proof.json> <selection.json> <仓外新proposal.json>
```

`prepare` 重验原 ZIP 字节和选择所绑定的原证明，仅输出 **unsigned-catalog-proposal**。`withdrawn` 记录撤回理由并保留原制品，不删除本地插件、原任务或历史。它不接受 `stable`、替换版本或原地覆盖；提案不是 TUF 元数据，不能直接当作可信目录导入。稳定晋级必须由维护者在签名/发布系统重新验实际制品与宿主证明，并通过独立 TUF 角色显式签名，见 [贡献流程](CONTRIBUTING.md)。

## 验证

新增Go预构建包的固定来源、原十包字节保留和实际宿主验收见[验证记录](docs/prebuilt-go-validation.md)；原创素材包的格式、权限和快照行为见[素材包验证](docs/asset-bundle-validation.md)。环境预设的字段和场景保存验证见[预设验证](docs/environment-preset-validation.md)。文本分析节点的固定版本及热更新验收见[节点动作验证](docs/node-action-validation.md)。外部worker包的固定来源和独立公开SDK消费者见[worker验证](docs/external-worker-validation.md)。[供应商候选验证](docs/provider-adapter-validation.md)记录25包的来源、旧制品保留和与宿主实际执行证据的分工。这些记录不授予签名或发布权。

`ORDIWEFT_PLUGIN_CLI` 和 `ORDIWEFT_PLUGIN_CLI_SHA256` 仅用于显式启用真实 CLI 打包/提案集成测试；未设置时相应测试 SKIP。无需模型 Key、Docker、Git 写 Token或 Morpheus。测试使用自己创建的临时副本，破坏性反例不修改仓内源码。

当前消费协议与完整平台计划见 [Ordiweft 插件契约](https://github.com/xurunxin/Ordiweft/blob/bd8bf8b417786cc8fe8bae3bd8fea79898be6cc8/docs/reference/plugin-release-contract.md)。新角色声明需要支持inputRoles的宿主；旧宿主严格拒绝未知字段，不忽略限制。静态process供应商需要支持该Host Services路径的宿主；包内源码由维护者受限示例构建器生成，portable-pack只打包其预构建字节。片场素材包需要支持director-models profile的宿主；旧宿主拒绝安装，不把GLB当JSON或执行入口。本仓portable-pack不执行Go或TS编译，任意工程构建由开发工具链提供完整制品。正式信任根、签名运维与官方稳定发布需维护者另行明确安排；本草稿不执行这些动作。
