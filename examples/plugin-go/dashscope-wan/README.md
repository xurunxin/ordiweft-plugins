# 独立百炼万相适配候选

Apache-2.0软件，只导入公开`pkg/pluginhost`，没有HTTP客户端、Key、业务数据库或Morpheus依赖。候选覆盖原`wan2.7-image`和`wan2.7-image-pro`模型定义：最多9张PNG/JPEG/WebP原图、组图上限12张、非组图最多4张及4K的原限制。稳定provider仍为`dashscope`；包ID为`ordiweft-examples/dashscope-wan`。

这是代表性适配候选，**不能替换完整百炼渠道**。Qwen图片、视频、语音、音色、区域配额等仍由原内置实现服务；尚未具备整个渠道的目录和行为对等证明。现有部署不自动迁移或删减目录。候选的软件许可不包含模型、账号、素材或供应商服务授权。

使用同一Host Services完成原模型上传策略查询、私有OSS multipart、一次异步生成、原票据checkpoint、有界查询及原响应选择的输出下载。上传策略及签名只留宿主，插件仅取得状态；原上传slot与callId的2xx回执是生成的事务前提，包括空正文成功响应。对象目录来自原回执，后缀来自原MIME，内容与SHA来自原资产。不同上传/生成映射共享对应效果槽，换操作名或callId不能再次提交。

`policies/`为不可执行的受审字段映射。`whenAny`从原参数/输入数/MIME匹配映射；不接受候选选择的上下文。`connection.profile.json`保留旧WORKSPACE_ID配置名以兼容现有设置，实际连接只引用服务端API_KEY与DASHSCOPE_URL，后者由部署方固定为所在地域/业务空间的完整API origin（含`/api/v1`）；不得从节点参数推断地域或workspace。profile不是权限。部署方须逐份绑定JSON策略、独立审查连接/OSS域名和预算，再运行宿主验证器。

临时OSS接口按同模型、同账号使用，没有跨任务缓存、过期刷新、上传重试或远端取消。官方当前将临时存储定位于开发测试；生产应使用另行接入并审查的持久OSS连接，不能把本示例的临时存储当作生产容量保证。原生渠道的既有路径保持。

```powershell
scripts/export-plugin-go-sdk.ps1 -Out <仓外新SDK目录>
node scripts/build-plugin-go-example.mjs --example dashscope-wan --sdk <SDK绝对路径> --sdk-commit <SDK固定提交> --out <仓外新目录>
ordiweft-plugin verify -root <输出目录>/bundle
ordiweft-plugin pack -root <输出目录>/bundle -out <仓外新ZIP>
```

构建关闭网络依赖解析及CGO，固定Linux/amd64入口；目标部署只消费预构建ZIP，不运行npm/Go构建。`fixtures/verification.json`是单张参考图、空上传响应、pending→成功和实际PNG的独立宿主验收输入，不授予网络或凭据。合成协议通过不能证明真实百炼访问、收费、画质或生产配额。

接口依据（2026-11-12核对）：[Wan2.7异步接口](https://help.aliyun.com/en/model-studio/wan-image-generation-and-editing-api-reference)、[原文件临时上传](https://www.alibabacloud.com/help/en/model-studio/get-temporary-file-url)。现有模型字段权威仍为宿主`internal/dashscope/catalog.go`，迁移对等测试核对两份原模型定义。
