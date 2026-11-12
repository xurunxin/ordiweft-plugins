# 公开供应商预构建候选

首批供应商候选共18包，仍为私有开发仓库、未签名/未发布候选。加入IndexTTS和百炼Wan时，原16包的236份源码/配方文件及全部原ZIP逐字节保留；没有覆盖版本或改写历史证明。publisher仍pending，不因本记录成为官方身份。Morpheus及其依赖不在分发闭包。后续GPT Image的第19包证据见下节。

| 包 | 固定源码 | release digest | ZIP SHA256 |
| --- | --- | --- | --- |
| indextts 1.0.0 | 0611461c1b2bcedb37474332ecc5c6368c6e9e1f | 220544b440fde864ea0f20d0f99fc2fd5c154cd82c1675f3ff722c5961bea1f6 | a4b2d30002b6daa8993800ab3cc7c02308bfad489451504db9cfaf0fa307d08d |
| dashscope-wan 1.0.0 | 9a9e059f7e41b6432ba360e34b2c4c24531ddd2f | af18d84d1089303e56821588f0bc51d6cc1bcfd44c61d6d5ab75cf7b15b5607c | b67db662b1a9e8a85f695fdc13bb384362521c43a196dbd82551c63d4a949777 |

各source完整保留原bundle：公开SDK、固定Go构建/工具链摘要、Go LICENSE/PATENTS与包Apache-2.0 LICENSE/NOTICE。IndexTTS为73份描述文件、Wan108份；SDK来源分别由各自原build-proof/sdk-export.json固定，不冒称同一宿主提交。

portable-pack使用干净归档6b46ba08854c3040f52237d2447fcb238a33cce8构建的宿主CLI，SHA256 `6d3d40178d94411716dcd65a5bfb5bdbff4e09b1a1555cb8b6b0d1f6fd9349f2`。实际check/build完成18份ZIP；新两份与上表原固定ZIP相同，旧16份与上一轮community-sixteen-build实际文件相同。v3证明保留逐包原来源，旧30c45ab工具来源放入retainedHostSources。没有在本仓编译、执行插件、安装依赖或调用模型。

真实CLI门启用后5项测试全部PASS、0 SKIP（12.074秒）：仓库/worker/process隔离元数据、不可变打包/提案、改源码和错误CLI摘要拒绝。另只准备了绑定原证明的两项unsigned-catalog-proposal，没有stable、TUF签名根、线上发布、合并或部署。

实际执行由宿主另验：IndexTTS五种模式、原票据与真实PCM；固定宿主6dc986a完整嵌入式应用完成显式停止/重启只读观察/原结果一次采用；Wan固定6b46ba0 HTTP消费者PASS62.38秒，原策略1/上传1/提交1/查询2，实际HTTPS PNG 288×288/7789字节、SHA不变、原字段和版本1保留。Wan使用独立协议模拟器及公开下载夹具，没有真实百炼推理；不是完整渠道/全应用部署或真实供应商验收。原生ID导入仍需部署方明确授权，模型关闭不会自动打开。

宿主当前证据：[IndexTTS](https://github.com/xurunxin/Ordiweft/blob/codex/plugin-platform-plan/docs/evidence/plugin-indextts-adapter-20261111.md)、[原任务只读恢复](https://github.com/xurunxin/Ordiweft/blob/codex/plugin-platform-plan/docs/evidence/plugin-readonly-recovery-20261112.md)、[百炼候选](https://github.com/xurunxin/Ordiweft/blob/codex/plugin-platform-plan/docs/evidence/plugin-dashscope-wan-20261112.md)。软件Apache-2.0不包含模型、音色、账号、素材或供应商服务许可；临时OSS仅按官方开发测试前提使用。完整供应商迁移、签名运维与整体平台仍在实施。

## 新检出与完整应用消费

固定 f568d0a951a9d322947eb7984859b35737e5eabe 从远程全新clone，使用原可信CLI重建18 ZIP与v3 build-proof，均逐字节相等；证明SHA256 `a34837bdb043e204466e9744258f85cef4c8073a48ab49d8e8842d4ada7822c8`。`community-eighteen-clean-clone-report.json` PASS，未执行包代码或发布。

完整Ordiweft宿主 e3912fe9b9e9ec5dc9e4b066fcaaf4b3b59f791e 从干净归档执行完整web构建并内嵌Linux二进制，以全新合成数据实例安装原Wan ZIP。原生ID显式授权、普通导入暂存、策略/连接、独立原验证和启用通过。原普通队列上传1/提交1/查询2，真实公开PNG 288×288/7789字节与固定SHA一致；标题/未知字段及一个版本保留，跨用户404，同原任务重放无新上游调用。首次容器缺席观察超时导致unknown，确认缺席后显式采用同原本地产物；没有新生成。随后同一二进制重启，原任务/画布只读恢复，计数不增。这是含显式原结果恢复的完整应用消费，不能写成首次无协调完成或真实百炼推理。产品与证据见宿主百炼候选文档。

## GPT Image与19包不可变重建

`ordiweft-examples/gpt-image/1.0.0`来源固定`21a80a4a52da18479f027c1c7f43cbc1ec9b4591`，公开SDK固定`ff65702dd5bf7cc9c3af2c660e15a373a092b2fc`，80份描述文件包含Apache-2.0/Go许可及干净构建证明。release为`51ca50593f0c09845d96ff5a863ce9b5ce51a0ae6763a4ecf2b27b9a1d248751`；ZIP 4560496字节，SHA256 `2c65d7491c30ac3b0cdac366c57be07a2159ad845a756cf44cbee03b4ba38d11`。

新可信CLI同样从21a80a4干净归档构建，SHA256 `9e458b09248247b2ce7f0133817c478a0b0d2240e1f7661bc5613592b6507b2d`。实际check/build共19 ZIP，旧18份与前轮独立clone的制品逐字节相同；5项真实CLI测试PASS、0 SKIP，14.133秒。构建只读取/打包预构建字节，不执行插件、安装依赖或发布。

宿主固定源码的实际Linux HTTP/SQLite/沙箱验证PASS120.02秒，覆盖同步/SSE/multipart、遮罩、同内容多输出以及异常SSE不重提。完整内嵌应用也实际安装本仓构建的ZIP：显式授权/原验证/激活后，五个普通原任务各提交一次，返回原17×13 PNG字节，标题/未知字段、结果数和版本数正确；重放没有额外调用，其他用户404。同一二进制重启后原任务/画布只读恢复，上游计数仍5。无宿主源码、开发依赖、Morpheus或真实账号挂载到运行应用；Docker控制仅在受信宿主，插件维持自己的无网络沙箱。不是整个GPT渠道、真实OpenAI调用或整体平台验收。

当前宿主证据：[GPT Image候选](https://github.com/xurunxin/Ordiweft/blob/codex/plugin-platform-plan/docs/evidence/plugin-gptimage-adapter-20261112.md)。全部候选仍未签名/未发布，不生成正式信任根或自动启用其他部署。

## SeedAudio与20包

新增ordiweft-examples/doubao-audio/1.0.0，源码固定eaf0d34be10c3aa9cb2e97451c56fbcee9462846，SDK固定ff65702，73份描述文件含公开SDK和Go/Apache许可。release 241fe84cee887a950c6a0435297e101a04018c5237c392f389ccda52c5541d0a；ZIP SHA256 b9d9df7bc95c2933292a4db555b943482a0613f70405e5930cf6e78f33d22e14。只迁移当前纯文本模型，不新增原生未提供的参考素材/音色。

可信CLI从eaf0d34干净归档构建，SHA256与前轮相同（9e458b09248247b2ce7f0133817c478a0b0d2240e1f7661bc5613592b6507b2d），实际重建20包，旧19份ZIP逐字节相等。五项真实CLI测试PASS、0 SKIP，15.414秒；不执行插件、安装依赖或发布。

固定Linux HTTP/SQLite/Docker消费者四项PASS97.54秒，覆盖PCM、显式WAV规范化来源、实际公网原URL音频和错误回执。完整独立内嵌应用也消费本仓原ZIP：授权/原验证/激活后四个原任务各提交一次，三项成功写入原字节和一个版本，错误保留unknown/零版本；标题/未知字段、原任务重放和其他用户404均通过。实际500ms自有PCM及203ms公网WAV以真实文件测量，不采信模拟上游时长。相同二进制PID19→90重启后，三份完成结果和原unknown仍可读，计数仍4、duplicate/invalid0。没有真实SeedAudio推理。

完整96MiB响应/64MiB音频组合、原追踪metadata及全渠道迁移仍未闭合，本候选不自动切换现有部署。公网音频只作仓外验证输入，未复制进包；包内fixture为自有合成PCM。宿主证据：[SeedAudio候选](https://github.com/xurunxin/Ordiweft/blob/codex/plugin-platform-plan/docs/evidence/plugin-seedaudio-adapter-20261112.md)。全部仍未签名/未发布，整体计划继续实施。

## SeedAudio原TraceID、容量和22包

新增不可变1.0.1与1.0.2；1.0.0及其余旧20份recipe/descriptor/ZIP逐字节保留。1.0.1固定源码84cb1eba7179dffc2cb42468e8ec0ed1c7a16e82、SDK8934478；ZIP SHA256 `8ae33ae32500f4337d4c7cb6444c05fd0975ffda27f43b76a7f405d4d5f49200`。1.0.2固定源码fde4f302c926bbd87e220b6c169525b4bc89de07、SDKf037588；release `d6299168609d27e645317dc0ed3966d33abbc6d633fca67b79f0c661229c5d1d`，ZIP SHA256 `de87aa6074e246ab4f8662320b5db6d42f53cd68473b61b8ea8f7c953bfc67c7`。新可信CLI来自8934478干净归档，SHA256 `7f06c29a4d235a8a95bdd2b96acb7ceb2cd4fecdf2e362e73285a69b1b2d2bc6`；旧CLI严格拒绝不认识的jobs.context，没有回退到非隔离执行。

宿主实际Linux HTTP/SQLite/受限Docker已验证1.0.0↔1.0.1活动generation1→2→3：六个原任务各一次提交，在途任务与等待隔离槽的已准入任务沿原release执行，unknown回执、凭据/模型开关、历史版本和其他用户404保留，旧激活命令重放不复活旧指针（PASS112.86秒）。这是等待物理执行槽，不冒充队列API的queued状态。保留Docker资源观察警告，异常资源终止故障门仍另验。

1.0.2在原256MiB子容器中完成67,108,860字节、192kHz双声道32bit WAV，SHA256 `a1c62d8a676feb653ef88fce59c24141c652a9518c14af24d9f0ade682466c93`，实际JobResult43690ms、原派生来源和画布写回正确。一次原提交、重放无额外调用、其他用户404，PASS64.25秒。宿主验证进程限2GiB，峰值内存未测量；音频输出上限仍64MiB。四条常规PCM/规范化WAV/实际公网URL/错误回执又在新包通过（84.84秒）。旧1.0.1大响应反例和测试夹具限额/时长断言失败均保留，不把反例或修正前断言算PASS。

实际check/build22包，新两包与宿主固定ZIP相同，旧20包metadata/ZIP逐字节相同。真实CLI的五项测试全部PASS、0 SKIP，17.312秒；仓库构建仍只打包已有字节，不安装依赖、不执行插件、不读取账号、不发布。宿主[容量及原任务证据](https://github.com/xurunxin/Ordiweft/blob/codex/plugin-platform-plan/docs/evidence/plugin-seedaudio-adapter-20261112.md)和[包间更新](https://github.com/xurunxin/Ordiweft/blob/codex/plugin-platform-plan/docs/evidence/plugin-package-hot-update-20261112.md)负责实际运行证明。完整独立新宿主、真实供应商与整体迁移继续分别验收；Apache-2.0不授予模型/账号/素材许可。
