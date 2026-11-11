# 开发验证

本仓库尚未正式发布。2026-11-11 使用固定 Ordiweft 源码 `732f0bafb581a05adab6ac8fd3cb28595b19f7fd` 构建的实际 CLI 验证，不运行插件代码、收费模型、签名或部署。

- PASS：4 个声明源码闭包的元数据检查，实际宿主 CLI 校验及打包。所有包随附 Apache-2.0 LICENSE/NOTICE；Morpheus 未包含。
- PASS：实际 CLI 集成测试 3 项，2.709 秒。分别打包两次，4 个 ZIP 的完整字节一致；提案固定原构建证明/制品摘要，禁止覆盖已有提案，拒绝稳定晋级、错误构建 SHA 和变化的 ZIP；撤回不改写或删除原制品。错误 CLI SHA 和修改后的源码由实际验证器拒绝。
- PASS：无依赖 package-lock，仅执行离线 `--package-lock-only --ignore-scripts`，不获取包或执行生命周期脚本。
- 验证工具和第一次4包构建记录保存在仓外 `D:/Projects/Ordiweft-validation/plugin-platform-live-20261110/`；不提交机器产物、路径配置或私钥。

NOT_RUN：通用依赖编译、TS/npm 隔离构建、真实供应商、生产、多后端宿主安装、本仓库正式 namespace 审批、TUF 签名/发布和官方稳定晋级。现有节点/processor/worker 完整交付继续由 Ordiweft 主计划跟踪。

源码闭包以 `.gitattributes` 的 `-text` 保持原字节，避免跨 Windows/Linux checkout 改变清单 SHA。仓库工具只准备未签名提案，不把本文件或可编辑 build-proof 当作独立可信审阅证明。

首次差异检查将保留的原 CRLF 当作尾随空白而 FAIL；已仅对不可变源码闭包声明 `cr-at-eol`，保留其他空白检查，不改写原发行字节。

## 文档/表单样本与来源分离增量

新增5份固定源码：form-node 1.0.0、document-text 1.0.1、media-inspection 1.0.1、document-file 1.0.0、document-normalize 1.0.0，来源均为Ordiweft 24a9024214700a743ccbdd4ea78f78215450dc28。原4份源码/配方继续固定732f0b，不改写旧版本。仍无依赖安装、插件执行或签名。

build-proof v2分别保存工具来源与逐包源码/配方SHA，prepare重验实际原来源；v1历史证明继续按旧共享提交范围读取，不补造原路径或配方指纹。配置的源码提交本身不认证CLI来源，维护者仍须独立检查真实构建及所给工具SHA。

- PASS：固定24a9024归档源码构建可信CLI，SHA256 f5ed2d2efe01cfa7912c997d31b18110764ebc188b5b3ed8c6c123c91a5ba52a；9包实际verify/pack成功，两次ZIP逐字节相同。与原community-clean-build的4个ZIP逐字节对比保持相同，并由原v1 build-proof准备4项历史提案，未改写旧证明。
- PASS：最终真实CLI集成3项，5.003秒，零SKIP。除原拒绝条件外，覆盖改源码提交后重新绑定选择SHA仍拒绝、混合新旧来源、v1历史读取与不能冒称较新源码，以及v1提案不补造路径/配方指纹。
- EXPECTED_FAIL：临时跳过逐包原源码比较，原集成主检查在错误来源提案被接受处断言失败（actual=0，3.704秒）；精确恢复后3项全通过5.134秒。随后仅收窄v1来源声明，最终再次通过5.003秒。

仓外制品与原证明位于plugin-platform-live-20261110/community-build-action-24a9024。实际沙箱、业务采用及停止证明属于Ordiweft主仓相应阶段，本仓打包不会重新执行；无费用、激活或正式发布。独立干净clone读取与打包在推送后另外记录。

9166469远程干净克隆的9包check/verify/pack通过，但完整构建证明对比FAIL：原4配方工作区CRLF与Git克隆LF使recipeSHA256不同。这是可移植证明的实际缺口，未将其记录成完整独立交付PASS；旧源码/ZIP未修改。

新增v3证明以JSON.parse后JSON.stringify的UTF-8字节生成recipeDigest，逐包原源码提交和真实ZIP SHA仍独立保存；v2继续读取真实原字节指纹，v1不补造来源。主集成扩展为两次构建之间只在临时副本转换配方CRLF→LF，并要求完整proof及9个ZIP相同，最终3项4.927秒PASS、零SKIP。新v3构建记录保存在community-build-v3-action-24a9024，不覆盖先前v2或v1记录。
