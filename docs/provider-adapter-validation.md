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

## 显式容量协商与原生编码兼容

新增1.0.3，固定源码ac3b7225c95a8289183712004111ad7d255d117b、公开SDKff4ef41a28532c453224f73dbde7ae3336efbbbe；release `741dd3ad36d1c15442228f1f9812af73e2580f515d9f28eba5708c7772b58a1b`，ZIP SHA256 `e5c4da871f3dae58c2bc4e20ce6f28cb6e622dbf0c137d1d91b93b295f4524b9`。每次audio解析显式给出maxSourceBytes=96MiB及json-base64-lenient，原严格调用保持严格；新路径只额外接受原生解析允许的CR/LF和非零padding bits，其他格式/字符/超限仍拒绝。旧8934478干净宿主在独立小fixture的assets.decode按预期拒绝、未激活或提交普通生成，47.79秒；避免小fixture误通过后大响应才失败。

新固定宿主实际七条PCM/WAV规范化/换行base64/非零padding/公网URL/真实自有MP3/错误路径PASS181.85秒，每任务一次提交、真实SHA/时长、原TraceID、版本、重放和404保持。MP3为仓外自有440Hz合成音，ffmpeg9.0.2生成，ffprobe识别44.1kHz单声道/8777字节/0.548563秒，业务时长按原口径548ms，SHA256 `c02bf0a1f8cba5f97a74dc9b94dbd3f36cbc46897d8f37041200f432c5dd49b4`；包内不再分发ffmpeg、其代码或该外部验证文件。近64MiB路径再次PASS70.81秒；原1.0.2不加新字段的固定包在新宿主也PASS70.67秒，保留缺省字段兼容。

仓库现23包，旧22份metadata/ZIP逐字节保留，新一包等于宿主固定ZIP；真实CLI五项PASS、0 SKIP，18.042秒。只做portable-pack，不执行插件或安装依赖；合成宿主更换二进制后保留原五任务/unknown/凭据与模型开关，新包原验证和generation1→2激活通过，完整八条路径及重启继续由宿主独立验收。许可与未签名/未发布范围保持，整体计划继续实施。

## GPT Image 1 系列与 24 包

新增不可变 `ordiweft-examples/gpt-image/1.0.2`，源码固定 `04ada2a0762dd33ccd36d224e5ffb5bf10ed0872`，公开 SDK 固定 `ff4ef41a28532c453224f73dbde7ae3336efbbbe`。包含原 gpt-image-1.5、gpt-image-1-mini 和 chatgpt-image-latest 三个模型；mini 继续只接受 low input_fidelity。八份 JSON 映射和连接引用同时固定摘要，继续共用原 generate 效果槽。原 1.0.0 保持；本地 1.0.1 因连接仍引用旧映射摘要被安装前检查拦住，保留原失败包，不收录为可安装候选。

新 release 为 `cc26d117c608e7d695ca85a5377f1f18e8e592943777f73c0aa51d5559a9713d`，ZIP SHA256 `640dd3d5e1530f1665d4cf54ee37b5b21a4eeca99153a9ec51aeb71e185e5079`。实际重建 24 ZIP，旧 23 份制品及 metadata 与 d8d0b46 的原证明逐字节相同，新 ZIP 与宿主固定包相同；可信 CLI 保持原 8934478 来源及既有摘要。五项真实 CLI 集成测试 PASS、0 SKIP，25.637 秒，没有执行插件、安装依赖或发布。

原生适配器实际拦截请求与三份公开 schema/八种受审映射的 24 组对照通过，包括同步/SSE、JPEG opaque、multipart 原参考图与遮罩、匿名身份及 mini 保真度。固定 cda532a51218ec2bb88c6b57fa737a903e441a08 Linux HTTP/SQLite/Docker 消费者对本包三模型共 21 条普通路径 PASS478.61 秒：15 个成功任务、6 个异常 SSE unknown，实际 18 份 PNG 输出、原提交一次、原字段/版本/回执重放/跨用户404保持。资源观察告警保留，不能据此关闭全部故障或性能门。

远端全新克隆9567d3f85e45d6bc3149b82f08af46bdbc5a2773重建24 ZIP及完整v3 proof均逐字节相同，proof SHA256 `d7cb77e4312b7ad272cec2cefb99f5980793e99b5ce25f85d16360b241537f70`，克隆保持干净。未签名单项候选提案绑定该原证明，不晋级stable或发布。

全新ac3b722完整内嵌应用从本仓同一ZIP安装，授权/原验证/激活阶段供应商提交0；三模型共15条同步/SSE/multipart/双图任务保存18份实际17×13 PNG，原字段/版本/回执/404通过，同二进制PID19→223重启后提交仍15、invalid/duplicate0。初次脚本在第14条遇到过长合成jobId的400；原13条没有重跑，确认未入队/未提交后只修正剩余两条标识，原失败与意图保留。另一旧实例的原验证缺独立判定，继续unknown，不换命令重跑；原五任务/设置、generation1、PID101及原供应商提交5保持。两实例证据不互相覆盖，见[宿主固定证据](https://github.com/xurunxin/Ordiweft/blob/6043693923c686ad7c0928636c2a5970a3fcd295/docs/evidence/plugin-gptimage-family-20261112.md)。

本包不替换现有完整渠道，不声称 GPT Image 2 系列、GIF/SVG/markup、大容量组合或真实供应商已完成迁移。全部仍为 unsigned/unpublished 候选，SDK/模板 Apache-2.0 与 Morpheus 原有许可边界保持。

## GPT Image 2与25包

新增不可变1.1.0，源码e766c033678bcd8939d0568a08bffe6b55f2ea62，公开SDK 0.1.13固定8c8841b79c59cd40e9aadc9f7b27fa24ba46aeaf，87份描述文件保留Apache-2.0/Go许可。release ebf3b7559fc15cc3eafdb57d3022a2b12ee0b6cb142dd3e7d58f93a9414a25b2，ZIP SHA256 708db99c9f342b5cc3007b028715e3cb5b3ca65afa6d334145c4e39520b6c999，4641194字节。13份受审映射覆盖当前11模型、56项预设、同步/SSE及SVG/GIF/标注；单底图标注保留image字段，多底图保留image[]。原1.0.0/1.0.2与其余历史版本不覆盖。

实际check/build25包，原24份ZIP/metadata与原证明逐字节相同，新的ZIP等于宿主固定包。完整v3 proof SHA256 aa02e4a87a7464db800762fc96f322a9c342c0864fcf3366a58293c1f8acb941；CLI继续使用8934478固定来源/既有摘要。五项实际CLI反例与不可变打包测试PASS、0 SKIP，27.898秒。本仓没有安装依赖或执行包代码。

宿主开发包11模型/89条实际Linux HTTP/SQLite/Docker队列路径PASS1339.09秒（63成功、26异常unknown、76媒体）；固定包86份运行文件和入口逐字节相同，只有来源证明与release描述不同，开发矩阵不冒充固定包矩阵。固定包另在8c8841b7完整嵌入式应用中同原数据消费：原15任务/设置保留，generation1→2，凭据不重设，9条新原任务/10份实际PNG/SVG/GIF通过；同二进制PID19→154重启后原15+新9可读，旧供应商仍15、新供应商仍9、invalid/duplicate0。安装报告变量异常保留，以GET原验证/激活回执恢复，不重跑安装或生成。

详见[宿主2系列证据](https://github.com/xurunxin/Ordiweft/blob/codex/plugin-platform-plan/docs/evidence/plugin-gptimage2-20261112.md)。高容量组合/并发峰值与真实供应商仍未完成，不自动切换实际部署或关闭整项迁移。全部仍为unsigned/unpublished候选，无正式信任根、合并、发布或部署；Morpheus不在本分发闭包。

远端全新clone ff89cc44b757d62b998665a9887c7f163bc60756实际重建25 ZIP及完整v3 proof，逐字节相同；clone保持干净，proof SHA256仍aa02e4a87a7464db800762fc96f322a9c342c0864fcf3366a58293c1f8acb941。单项候选选择另绑定该原证明准备unsigned proposal，不晋级stable或发布。

## GPT Image 1.1.4 与第26个候选

固定源码b49db592c2c6d2dc3c00b0e995a67fddd218008c、公开SDK/可信CLI bd8bf8b417786cc8fe8bae3bd8fea79898be6cc8、固定Node24镜像。91文件预构建包release326c789a85688a70f4e1eab6dd71b31e1b2f8c2f98605697fe2a358139837160，ZIP SHA2569da0db9e344b776e765326564165993b6f31123123b731c70a60799fe6df2482，4,777,147字节。新包要求显式sandbox.scratchBytes和outputs.stage-buffer；旧严格宿主/CLI拒绝，不默默忽略资源声明。

实际完整宿主18609153通过三张各50MiB同步PNG与单张50MiB SSE（150.23秒），原一次提交/真实SHA/像素/字节/版本与404保持；36MiB的同步/SSE/multipart四路径另在1.1.3通过，不能拿其PASS覆盖原50MiB OOM。1.1.4同原完整应用保留24条旧任务/设置，新增9路径10份媒体、PID19→152重启后均保持；同PID的包指针generation3→4→5回退/恢复保留33条任务/文档，旧命令重读不改变当前指针。回退未生成旧包新任务，不宣称GPT在途窗口。原供应商计数15+9+9不变，真实供应商未调用。旧unknown/32MiB暂存失败/1.1.3 OOM各保留原记录。固定[完整范围](https://github.com/xurunxin/Ordiweft/blob/66aa1944/docs/evidence/plugin-gptimage-streaming-20261112.md)与[迁移盘点](https://github.com/xurunxin/Ordiweft/blob/66aa1944/docs/reference/plugin-migration-inventory.md)继续区分全部原生多输入/高像素遮罩、并发与真实账号未验收。

本仓实际CLI五项PASS/零SKIP（30.114秒），首次启动器替换错误导致3PASS/2SKIP的日志保留，不拿跳过当集成通过。新可信CLI SHA256ac7a9a07840404763380590f9df93ccbd74ca5d12845fb069d63c55575ae37af实际验证并打包26份；与原25份逐项比较，artifact JSON序列化和ZIP字节均相同。新v3 proof SHA256d7eb89f74ef6c41fca3914960b17b0ec8b1908bda402b8c110fef825714de2ac；继承原1.1.0候选决定，只追加1.1.4，生成2项unsigned提案，没有改旧决定或签名根。远端冷clone d8e473190c45e2cddcb5eae61eb93d105b7052c4的26 ZIP和完整v3 proof实际逐字节全等（PASS），proof SHA同上；该步骤为portable-pack，未把打包冒充Go编译或插件执行。
