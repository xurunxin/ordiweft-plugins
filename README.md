# Ordiweft 社区插件源码仓库

保存可审阅源码、固定配方与发布提案，使独立部署可以通过 Ordiweft 的签名目录或离线包消费插件。当前为私有开发仓库；内容尚未发行，也没有官方信任根、线上目录、自动合并或部署。

原创工具与模板采用 [Apache-2.0](LICENSE)，保留 [NOTICE](NOTICE)；每个包携带自己的许可与来源。Morpheus 保持其现有许可，本仓库不分发 Morpheus 或其依赖。

## 当前可用范围

四个无依赖示例来自固定的 Ordiweft 提交：JSON 内容、可填提示词模板、显式 Skill、离线固定图片 Node 适配器。源码及 release.json 的完整文件清单一起保存。它们保留原 package/provider ID，未经命名空间审核的示例不会自称官方适配器。

`portable-pack` 仅校验和打包预构建字节，不执行插件、npm 脚本、依赖安装或模型请求。它不是通用 TypeScript/npm 编译沙箱。所需 Node 运行镜像另固定摘要，安装时仍由部署管理员决定是否具备相应运行器。

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

输出 ZIP 与 build-proof.json。证明记录实际制品 SHA/字节数、宿主 release digest、选择的 CLI 摘要及配置的源码提交；它是待复核的构建记录，不是签名、命名空间授权或供应链证明。所有输出不覆盖已有文件，失败保留现场供检查。文件完整性以真实宿主 CLI 为准，不在这里复制另一份 release 摘要算法。

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

`ORDIWEFT_PLUGIN_CLI` 和 `ORDIWEFT_PLUGIN_CLI_SHA256` 仅用于显式启用真实 CLI 打包/提案集成测试；未设置时相应测试 SKIP。无需模型 Key、Docker、Git 写 Token或 Morpheus。测试使用自己创建的临时副本，破坏性反例不修改仓内源码。

当前消费协议与完整平台计划见 [Ordiweft 插件契约](https://github.com/xurunxin/Ordiweft/blob/732f0bafb581a05adab6ac8fd3cb28595b19f7fd/docs/reference/plugin-release-contract.md)。完整编译构建、社区审阅自动化、正式签名运维与官方稳定发布仍在实施。
