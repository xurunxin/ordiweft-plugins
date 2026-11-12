# Go 原文档统计

原创 Apache-2.0 示例，使用公开 Go SDK 的 OpenAction/ReadAsset/Complete，经宿主原素材接口读取一份 UTF-8 纯文本，返回字符数、空白分词数量及原文摘要。不联网、不调用模型、不写画布或生成作业。分词是 strings.Fields 的确定性空白切分，不声称中文语义分词。

此目录只保存源码与元数据模板。受信开发者使用已安装工具链和固定提交导出的公开 SDK，在仓库外构建不可变 Linux/amd64、CGO=0 制品：

```powershell
pwsh.exe -NoProfile -File scripts/export-plugin-go-sdk.ps1 -Out <新的SDK目录>
node scripts/build-plugin-go-example.mjs --sdk <SDK绝对目录> --sdk-commit <SDK导出提交SHA> --out <仓外的新输出目录>
go run ./cmd/ordiweft-plugin verify -root <输出目录>/bundle
go run ./cmd/ordiweft-plugin pack -root <输出目录>/bundle -out <输出目录>/document-count.zip
```

构建器只编译本例的已审阅固定源码，不是任意社区源码的安全构建服务。模块代理、sum服务及自动工具链下载关闭，不运行 go generate、安装 hook 或编译后的入口。生产安装只消费完整清单中的预构建 adapter；不得在宿主上启动该二进制。社区/开发台的通用隔离编译仍须另行接入。

bundle包含入口、原Go源码、固定公开SDK及其许可/逐文件导出清单、独立动作fixture和构建来源，也保留同一Go工具链的LICENSE/PATENTS。Go模块源码与宿主internal/Morpheus分开；SDK的Apache-2.0不重新许可Go运行时、宿主或Morpheus。证明记录构建器与Go/compile/link摘要，不宣称冻结了整个开发机或标准库源码；它不是签名、发布或功能测试证明。实际隔离、权限、原回执和独立验证由宿主完成。

当前模板为1.0.1，使用更新后的固定SDK与受限示例选择构建器；旧1.0.0制品及证明保留，不能用新编译字节覆盖。构建参数省略--example时仍选择本例。
