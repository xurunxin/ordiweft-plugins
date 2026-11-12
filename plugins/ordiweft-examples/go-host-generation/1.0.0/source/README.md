# Go 宿主图片供应商

原创 Apache-2.0 示例，通过公开 Go SDK 的 OpenGeneration/Host.Call/Complete 调用宿主的 generate 操作、保存原票据、解析 JSON-base64 图片并采用原产物。模型、参数映射、地址和凭据由管理员安装的宿主策略决定，代码不读取环境密钥或自行发HTTP请求。示例不报告虚构的用量；真实计量应从原供应商证据提取。

受信维护者使用已安装工具链和固定公开SDK，在仓外构建固定静态Linux/amd64制品：

```powershell
pwsh.exe -NoProfile -File scripts/export-plugin-go-sdk.ps1 -Out <新的SDK目录>
node scripts/build-plugin-go-example.mjs --example host-generation --sdk <SDK绝对目录> --sdk-commit <SDK提交SHA> --out <仓外新目录>
go run ./cmd/ordiweft-plugin verify -root <输出目录>/bundle
go run ./cmd/ordiweft-plugin pack -root <输出目录>/bundle -out <输出目录>/go-host-generation.zip
```

构建器只选择仓内已审阅的固定示例，不接受任意源码路径，不下载依赖、不运行go generate或编译后的入口。完整包保留源码、固定SDK/清单、Go运行时LICENSE/PATENTS和构建来源。维护者构建工具不是后台Agent的通用安全编译服务；Morpheus不在发行闭包中。

在管理后台暂存包，安装名为generate的请求策略和宿主连接，再运行合成验证。策略需要将demo映射到管理员选择的实际模型，固定model/n及可选prompt；连接定义限制原点、POST路径、API_KEY引用和请求/响应大小。不要把真实地址或密钥加进本源码包，使用[宿主接入指南](../../../docs/how-to/plugin-sdk.md)。

包内fixture返回固定2×2 PNG，不访问已安装上游，也不证明真实供应商支持该协议。验证通过后显式启用，并在渠道设置中开启；真实供应商兼容性另验。所有调用ID固定在原任务内，缺失回包或原结果未知时保留原记录，不通过新ID或再次运行来补齐。

更新使用相同package/provider ID并提升版本，保留已安装旧字节及在途任务。原有SDK和受信stdio路径继续独立使用；本例只请求Host Services，不授予任意连接、文件或业务写入权限。
