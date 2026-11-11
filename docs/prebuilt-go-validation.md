# 静态 Go 预构建包验证

当前十二包仍为未发布、未签名候选。新增go-host-generation 1.0.0和go-document-count 1.0.1，不改写原十包源码、配方或ZIP。命名空间保持pending，不因这份记录获得官方身份。

两个Go制品由Ordiweft固定源码`cfcf2dd01d145bad1d2d3b33ffd9fcdb820e0e12`受限示例构建器生成，SDK固定`7c172bab4a93663908aa531c35a4327c50b9f412`的51文件Apache-2.0导出，sourceDirty=false。独立干净检出重建后，供应商63文件和处理器64文件、入口、release.json及完整build-proof逐字节一致。Go运行时LICENSE/PATENTS单独保留，未包含Morpheus。

宿主CLI由固定cfcf2dd构建，SHA-256为`5b56bfb70bd978023d9a29ed7c64cea6e7685ef90959ff42d6835001894e7392`。实际CLI检查和打包完成十二份ZIP；新两包与宿主规范ZIP完全相同，原十包与先前b11ef92工具构建的原ZIP完全相同。新包release digest分别为：

| 包 | release digest |
| --- | --- |
| go-host-generation 1.0.0 | a3f43c85ed6f8d3bae615692f4a75924c9ebb7c92b925b084e39460bbc4a5b2b |
| go-document-count 1.0.1 | 70b25cf5505193fde8c83e92ee33e401bc52f90fdfe42062dbc50907c50204c1 |

真实CLI门开启后4项测试全部通过（7.944秒），覆盖重复打包/配方换行、旧证明/版本撤回、不覆盖、改源码/CLI摘要拒绝及process缺少固定镜像拒绝。没有编译或执行包代码、依赖安装、模型请求、秘密读取或签名。

另由宿主执行干净制品：文档处理器35.72秒、公开图片供应商17.92秒，包53.740秒通过；含原96KB文档/摘要、独立合成验证、原图片引用、标准队列、原结果重读与原制品可用性恢复，原业务上游一次。供应商没有虚构用量记录。该验收与本仓portable-pack分开，不把静态打包称为执行或真实供应商验证。

两份原构建绑定的unsigned候选提案已准备，没有stable晋级、TUF根、发布、生产部署或自动合并。独立远程检出、更多部署/迁移与故障矩阵继续由宿主实施计划核对。
