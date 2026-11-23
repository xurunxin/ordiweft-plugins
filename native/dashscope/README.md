# 阿里云百炼 · 受信原生适配器

本目录维护完整的 Go 适配器及合成回归用例。首次导入固定于宿主提交 8ebf4396ad9f9c87fb280e3be9f518cdf6f5a4b9。原运行时 ID 为 dashscope，原模型、节点、区域计价、工作空间、官方音色目录、复刻缓存和异步任务回执由宿主按同一契约管理。

此模块保留既有 Go 导入路径，由 Ordiweft 固定源提交后静态链接。构建时使用宿主提供的 AIGC SDK、百炼官方音色快照和只读模型目录工具。模块内容可在宿主隔离工作树按固定提交核对后编译和验收。执行生产切换需要受信宿主构建。既有 dashscope-wan ZIP 仍作为单独的万相示例维护。

在使用此源码的 Ordiweft 工作树运行：

```powershell
go test gitea.nkit.top/xurx/Ordiweft/internal/dashscope ./internal/voices ./internal/httpapi
go build ./cmd/ordiweft
```

真实账号查询、提交和收费调用分别使用授权的验收流程。
