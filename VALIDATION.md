# 开发验证

本仓库尚未正式发布。2026-11-11 使用固定 Ordiweft 源码 `732f0bafb581a05adab6ac8fd3cb28595b19f7fd` 构建的实际 CLI 验证，不运行插件代码、收费模型、签名或部署。

- PASS：4 个声明源码闭包的元数据检查，实际宿主 CLI 校验及打包。所有包随附 Apache-2.0 LICENSE/NOTICE；Morpheus 未包含。
- PASS：实际 CLI 集成测试 3 项，2.709 秒。分别打包两次，4 个 ZIP 的完整字节一致；提案固定原构建证明/制品摘要，禁止覆盖已有提案，拒绝稳定晋级、错误构建 SHA 和变化的 ZIP；撤回不改写或删除原制品。错误 CLI SHA 和修改后的源码由实际验证器拒绝。
- PASS：无依赖 package-lock，仅执行离线 `--package-lock-only --ignore-scripts`，不获取包或执行生命周期脚本。
- 验证工具和第一次4包构建记录保存在仓外 `D:/Projects/Ordiweft-validation/plugin-platform-live-20261110/`；不提交机器产物、路径配置或私钥。

NOT_RUN：通用依赖编译、TS/npm 隔离构建、真实供应商、生产、多后端宿主安装、本仓库正式 namespace 审批、TUF 签名/发布和官方稳定晋级。现有节点/processor/worker 完整交付继续由 Ordiweft 主计划跟踪。

源码闭包以 `.gitattributes` 的 `-text` 保持原字节，避免跨 Windows/Linux checkout 改变清单 SHA。仓库工具只准备未签名提案，不把本文件或可编辑 build-proof 当作独立可信审阅证明。

首次差异检查将保留的原 CRLF 当作尾随空白而 FAIL；已仅对不可变源码闭包声明 `cr-at-eol`，保留其他空白检查，不改写原发行字节。
