# TapNow MCP

本目录维护 TapNow MCP 协议、工具发现、OAuth 及提交回执逻辑。客户端随受信宿主编译，并由 `tapnow-host-v1` 发行包接管渠道 manifest。宿主管理账号、费用、用户权限、队列与资产写入。工具 schema 由当前 MCP 连接发现。

在 Ordiweft 的固定源码镜像中运行 `go test ./internal/tapnow ./internal/httpapi`。接管验收使用合成供应商；真实生成需要单独授权。
