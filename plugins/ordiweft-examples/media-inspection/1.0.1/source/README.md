# 素材检查处理器

原创Apache-2.0样本，使用同包vendored公开SDK。管理员导入并显式激活后，成员通过画布范围plugin-actions API提交一个获准的image/audio资产引用和空arguments。宿主固定原字节元数据并提供本任务句柄；样本调用共享media.inspect和有界assets.read，返回JSON检查候选，不生成媒体或修改画布。

运行要求部署已安装的固定Linux amd64 Node镜像，完整协议窗口60秒，资源清理另行确认。没有网络、凭据、路径、输出文件、派生或文档采用权限；输入撤权后缓存读取及结果采用仍被拒绝。静态导入ready不是功能验证证书。

1.0.1新增列入完整清单的fixtures/action-verification.json：固定1×1 PNG、空参数和实际媒体元数据/前8字节预期值。后台暂存后可运行独立动作验证，不使用业务素材或供应商连接；原完整结果、宿主比较及资源释放确认后才提交就绪。Morpheus四个公开authoring工具复用此验证器，通过不激活，停止后不迟到就绪，unknown只核对原结果。

```powershell
go run ./cmd/ordiweft-plugin verify -root examples/plugin-node/media-inspection
go run ./cmd/ordiweft-plugin pack -root examples/plugin-node/media-inspection -out <新的仓外ZIP路径>
```

输出来源是实际字节检查，不代表转写、语义分析、OCR、文档转换或外部渲染worker已经实现。
