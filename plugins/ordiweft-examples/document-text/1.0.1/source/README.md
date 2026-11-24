# 结构文档转文本

原创 Apache-2.0 纯计算样本，将固定的 title/paragraphs JSON 转为段落文本候选。没有网络、凭据、素材句柄、输出文件或业务写入；不调用模型、不使用生成记账。它不是通用 PDF/Word/OCR 解析器。

包内固定的 host.mjs 来自同许可 SDK 0.1.3，使用openAction读取动作帧并在完成后关闭私有流；不手写任务重连或接管其他输入输出。SDK可表达的worker/句柄字段不代表本样本或宿主已授予这些能力。

样本声明宿主现有的60秒执行窗口，包含容器启动和协议结束等待；代码/包校验及单独资源清理另有有界窗口。当前Windows/Docker桥接曾在较短10秒窗口里收到合法终帧后仍超时，不能据此确认成功或重提。完整协议、进程成功退出和资源退出的证明仍分别检查。

安装要求部署已经配置固定 Linux amd64 Node 镜像和独立 runner ID，不自动拉镜像或在宿主执行代码。`ordiweft-plugin verify -root <目录>` / `pack -root <目录> -out <新ZIP>` 检查和打包；普通导入静态校验后就绪，管理员仍须显式激活。暂存接口只保存 staged，不激活。

1.0.1新增固定的fixtures/action-verification.json。通过后台暂存后，可在「候选运行验证」独立运行Title/Body合成参数并比较原JSON结果。验证复用真实动作协议和资源账本，通过后仅就绪；不需要供应商连接，也不创建生成作业。它是合成契约用例，不证明任意文档质量；断线只读原命令，unknown不重跑。

POST `/api/canvases/{id}/plugin-actions` 使用固定 commandId、releaseId、portId=`document-text` 和 arguments=`{title,paragraphs}`。相同命令只读取原任务；当前仅本人且有当前画布权限可读取。返回值是候选，不改变画布正文或素材库。GET 原任务与显式 reconcile 不重新执行，cancel 阻止采用且保留执行槽至资源实际退出。

升级使用新包版本和文件摘要。现有任务保留原代码、定义和结果；本例不授权 npm 安装、任意 UI 或远程 worker。
