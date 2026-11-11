# 公开供应商预构建候选

本次共18包，仍为私有开发仓库、未签名/未发布候选。新加IndexTTS和百炼Wan，原16包的236份源码/配方文件及全部原ZIP逐字节保留；没有覆盖版本或改写历史证明。publisher仍pending，不因本记录成为官方身份。Morpheus及其依赖不在分发闭包。

| 包 | 固定源码 | release digest | ZIP SHA256 |
| --- | --- | --- | --- |
| indextts 1.0.0 | 0611461c1b2bcedb37474332ecc5c6368c6e9e1f | 220544b440fde864ea0f20d0f99fc2fd5c154cd82c1675f3ff722c5961bea1f6 | a4b2d30002b6daa8993800ab3cc7c02308bfad489451504db9cfaf0fa307d08d |
| dashscope-wan 1.0.0 | 9a9e059f7e41b6432ba360e34b2c4c24531ddd2f | af18d84d1089303e56821588f0bc51d6cc1bcfd44c61d6d5ab75cf7b15b5607c | b67db662b1a9e8a85f695fdc13bb384362521c43a196dbd82551c63d4a949777 |

各source完整保留原bundle：公开SDK、固定Go构建/工具链摘要、Go LICENSE/PATENTS与包Apache-2.0 LICENSE/NOTICE。IndexTTS为73份描述文件、Wan108份；SDK来源分别由各自原build-proof/sdk-export.json固定，不冒称同一宿主提交。

portable-pack使用干净归档6b46ba08854c3040f52237d2447fcb238a33cce8构建的宿主CLI，SHA256 `6d3d40178d94411716dcd65a5bfb5bdbff4e09b1a1555cb8b6b0d1f6fd9349f2`。实际check/build完成18份ZIP；新两份与上表原固定ZIP相同，旧16份与上一轮community-sixteen-build实际文件相同。v3证明保留逐包原来源，旧30c45ab工具来源放入retainedHostSources。没有在本仓编译、执行插件、安装依赖或调用模型。

真实CLI门启用后5项测试全部PASS、0 SKIP（12.074秒）：仓库/worker/process隔离元数据、不可变打包/提案、改源码和错误CLI摘要拒绝。另只准备了绑定原证明的两项unsigned-catalog-proposal，没有stable、TUF签名根、线上发布、合并或部署。

实际执行由宿主另验：IndexTTS五种模式、原票据与真实PCM；固定宿主6dc986a完整嵌入式应用完成显式停止/重启只读观察/原结果一次采用；Wan固定6b46ba0 HTTP消费者PASS62.38秒，原策略1/上传1/提交1/查询2，实际HTTPS PNG 288×288/7789字节、SHA不变、原字段和版本1保留。Wan使用独立协议模拟器及公开下载夹具，没有真实百炼推理；不是完整渠道/全应用部署或真实供应商验收。原生ID导入仍需部署方明确授权，模型关闭不会自动打开。

宿主当前证据：[IndexTTS](https://github.com/xurunxin/Ordiweft/blob/codex/plugin-platform-plan/docs/evidence/plugin-indextts-adapter-20261111.md)、[原任务只读恢复](https://github.com/xurunxin/Ordiweft/blob/codex/plugin-platform-plan/docs/evidence/plugin-readonly-recovery-20261112.md)、[百炼候选](https://github.com/xurunxin/Ordiweft/blob/codex/plugin-platform-plan/docs/evidence/plugin-dashscope-wan-20261112.md)。软件Apache-2.0不包含模型、音色、账号、素材或供应商服务许可；临时OSS仅按官方开发测试前提使用。完整供应商迁移、签名运维与整体平台仍在实施。
