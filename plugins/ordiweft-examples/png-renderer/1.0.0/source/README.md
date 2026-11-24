# 外部色块渲染

原创 Apache-2.0 声明式worker包，产生1–64像素宽高的PNG色块候选。包不包含网络端点、凭据或可执行入口，不自动安装渲染服务；由管理员独立部署受信worker并固定原release的端点与实现摘要，然后显式激活。

成员从处理器工具填写宽、高和颜色后明确运行。结果只是私有附件，不自动写入画布；断线只核对原任务，不重提。热更新不改原任务或已有文件。配套独立Go worker与部署边界见上级worker目录。

包目录中执行 `ordiweft-plugin verify --root .`，再以 `ordiweft-plugin pack --root . --out ../png-renderer-1.0.0.zip` 输出到包外新路径。静态ready不代表远端功能通过。
