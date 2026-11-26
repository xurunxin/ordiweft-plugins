# TapNow Web

本目录维护 TapNow Web 的网页目录、模式与参数转换、账号刷新、报价、生成票据及回读逻辑。客户端随受信宿主编译，并由 `tapnow-host-v1` 发行包接管渠道 manifest。宿主管理 Web 账号、费用、用户权限、队列与资产写入。既有节点和远端票据继续使用原渠道标识 `tapnow-web`。

在 Ordiweft 的固定源码镜像中运行 `go test ./internal/tapnowweb ./internal/httpapi` 和 `node --test scripts/tapnow-transform-fingerprint.test.mjs`。使用合成账号与 API；真实生成需要单独授权。
