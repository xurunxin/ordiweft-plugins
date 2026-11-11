# Ordiweft 社区插件源码仓库

保存可审阅源码、固定配方与发布提案，使独立部署可以通过 Ordiweft 的签名目录或离线包消费插件。当前为私有开发仓库；内容尚未发行，也没有官方信任根、线上目录、自动合并或部署。

原创工具与模板采用 [Apache-2.0](LICENSE)，保留 [NOTICE](NOTICE)；每个包携带自己的许可与来源。Morpheus 保持其现有许可，本仓库不分发 Morpheus 或其依赖。

## 当前可用范围

十六份无外部依赖的预构建包分别固定原 Ordiweft 源码提交：JSON 内容、可填提示词模板、显式 Skill、表单节点、离线固定图片 Node 适配器，以及结构文本、素材检查、私有文档文件、原文档换行规范化处理器。规范化同时保留1.0.0和声明原素材用途的1.0.1，用于同包升级与独立版本撤回。源码及 release.json 的完整文件清单一起保存。它们保留原 package/provider ID，未经命名空间审核的示例不会自称官方适配器。静态Go文档处理器1.0.1和Host Services图片供应商1.0.0另包含固定公开SDK、Go运行时许可和原构建证明；新增片场素材包携带原创Apache-2.0三角面，成员显式导入个人/团队模型库，环境预设另提供既有灯光和地面设置的显式套用，保留背景与场景值快照；新增文本分析动作节点固定同包原处理器，显式打开共享试参表单；更新不改原节点或原任务，新增PNG worker包只分发无素材输入的声明式ActionSpec，固定部署端点、实现及凭据引用由宿主管理员另行绑定；它不携带独立服务、运行镜像或启动命令。已有十五份源码、配方及制品字节不改写。

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

新增Go预构建包的固定来源、原十包字节保留和实际宿主验收见[验证记录](docs/prebuilt-go-validation.md)；原创素材包的格式、权限和快照行为见[素材包验证](docs/asset-bundle-validation.md)。环境预设的字段和场景保存验证见[预设验证](docs/environment-preset-validation.md)。文本分析节点的固定版本及热更新验收见[节点动作验证](docs/node-action-validation.md)。外部worker包的固定来源和独立公开SDK消费者见[worker验证](docs/external-worker-validation.md)。这些记录不授予签名或发布权。

`ORDIWEFT_PLUGIN_CLI` 和 `ORDIWEFT_PLUGIN_CLI_SHA256` 仅用于显式启用真实 CLI 打包/提案集成测试；未设置时相应测试 SKIP。无需模型 Key、Docker、Git 写 Token或 Morpheus。测试使用自己创建的临时副本，破坏性反例不修改仓内源码。

当前消费协议与完整平台计划见 [Ordiweft 插件契约](https://github.com/xurunxin/Ordiweft/blob/bd2e58919594d0c9aafabc357cedc95d2ef5039e/docs/reference/plugin-release-contract.md)。新角色声明需要支持inputRoles的宿主；旧宿主严格拒绝未知字段，不忽略限制。静态process供应商需要支持该Host Services路径的宿主；包内源码由维护者受限示例构建器生成，portable-pack只打包其预构建字节。片场素材包需要支持director-models profile的宿主；旧宿主拒绝安装，不把GLB当JSON或执行入口。完整不受信编译构建、社区审阅自动化、正式签名运维与官方稳定发布仍在实施；本仓portable-pack不执行Go或TS编译。
