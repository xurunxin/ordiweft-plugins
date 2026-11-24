# UTF-8 文档规范化 1.0.1

原创 Apache-2.0 样本，使用公开 SDK 0.1.5。此版本显式声明 source（原文档）角色：一份 text/plain 文档。宿主从实际字节及当前 ACL 校验格式和角色，不信任扩展名。保留原件，把 CRLF/CR 转为 LF，返回私有附件及原文摘要；执行不写画布、不联网。

`document-normalize/` 中原1.0.0示例保持原字节；本目录是新版本，须使用支持 inputRoles 的宿主。旧宿主严格拒绝未知元数据，不忽略角色限制。源码打包无安装脚本或外部依赖；LICENSE/NOTICE 与固定 host.mjs 一并纳入清单。

从仓库根执行：

```powershell
go run ./cmd/ordiweft-plugin verify -root examples/plugin-node/document-normalize-roles
```
