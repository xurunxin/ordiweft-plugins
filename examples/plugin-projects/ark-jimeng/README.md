# 火山方舟独立生成包

2.2.2 候选通过支持素材续写的宿主提供 Seedance 2.5 和 2.5 Flash 的「用视频续写」。新节点固定原视频并选择 extend，比例保持 adaptive，使用已有方舟 API 延长路径。Seedance 2.5 的「草稿转正片」沿用原样片回执和对应渠道，输出 1080p。此版本使用 fromDraft.actionLabel 声明按钮文案。
当前 2.2.1 候选增加 Seedance 2.5 Flash（`doubao-seedance-2-5-flash-261115`）。模型支持 4–30 秒、480p/720p，首尾帧、视频编辑和延长由方舟 API 执行。需要为方舟 API 启用时，将该模型 ID 加入渠道的 `models` 配置。

2.2.0 候选与渠道0.5.0协商`generation.image-review@1`、`channel.image-review@1`，primary连接显式启用imageReview。素材审核入口按下游渠道出现在源图片右键菜单中；状态呈现在对应连线上，未审核不显示盾牌。官方方舟素材与即梦、TapNow的账号空间和结果分别保存。使用及验收范围见[素材审核操作](../../../docs/how-to/ark-jimeng-plugin.md#素材入库与审核)。未发布或部署该候选。

2.1.4引入并继续保留的候选只提供普通图片／视频入口，channel默认auto。音频字段不再注入true，文件格式／背景默认跟随渠道，提示词优化不再自动配置。旧明确参数保留，未支持的明确要求仍使用API；默认兼容请求实际走CLI。候选要求渠道0.4.5及channel.default-parameters@1，旧服务拒绝新默认语义；详见[默认路由复审](../../../docs/evidence/plugin-default-route-285.md)。

继承2.1.2的独立queryOnly报价策略和quote连接，要求宿主generation.quote@1及渠道0.4.3的channel.readonly-quote@1；0.4.4服务补充有审阅期的官方API标准价预估。页面刷新不生成、上传或node finalize。SDK0.1.15与私有工具0.2.3精确固定；原2.1.2及旧ZIP保留。内部审计另由宿主daily-v1按冻结日常价核算，报价不当成实扣或日常价。见[当前验收](../../../docs/evidence/plugin-ark-api-quote-285.md)。

2.1.1要求渠道0.4.2新增的报价空间与提交前栅栏能力，原2.1.0候选保留。确定未调用生成命令的本地拒绝可以结束本地CLI槽；原任务不会自动重试，丢失生成响应仍为unknown。真实只读预检、会员和有限预算见[#285复审](../../../docs/evidence/plugin-jimeng-video-review-285.md)。

保留原生实现和历史任务，须经native迁移预检/授权、配置连接、合成验证及显式激活。连接只指向管理员配置的独立渠道服务；包没有Key、CLI或网络权限。只读恢复使用原连接和回执。旧ZIP不重写。

当前候选仍在验收，不构成正式发布。fixtures内包含待管理员审阅的连接策略模板、JSON策略来源及已装配的connection.json。策略摘要来自公开Go SDK的JSONRequestPolicy.Digest；修改策略后须重新计算并装配，不能直接把模板占位字符串当作可用配置。CHANNEL_URL和CHANNEL_TOKEN仅由宿主端环境引用或加密保存；API_KEY保留旧渠道配置兼容，隔离包不读取它。

2.1.1显式声明宿主runtime/channel协商、outputCount、MP4 inspect及transparent事实能力，固定SDK 0.1.15并要求渠道至少0.4.2的新增视频参数／不可变Draft语义。连接采用`/v1/tasks`和认证的`/v1/capabilities`，不支持旧未协商的新提交。插件在任何连接提交前核对原素材SHA、实际尺寸、时长及模式组合；透明像素未知时拒绝需要透明参考的请求。视频输入仍受平台单项64MiB限制，且须先有服务器已授权分发的HTTPS CDN原件地址；不冒称支持供应商的全部200MB上限。

`seedream-5.5-auto`只声明图片描述、1.5K/2K尺寸、比例、水印及渠道。`size/aspect_ratio`映射为CLI的`resolution/ratio`，提交前以CLI实际模型目录再确认支持范围。新建图片和视频节点默认关闭水印；即梦账号开关须预先关闭，驱动本身不修改账号，返回格式按实际字节识别；需要指定PNG、透明背景、拆层或其他旧参数时，继续选择原模型并使用API。模型参考报价中的`affordable=false`若没有余额，不作为积分不足证据；另读同账号空间的`credit get`余额与已知参考价比较，未知则停止。报价只是估算，生成后的实际扣点以原任务结果和余额记录为准。

构建使用单独固定的私有维护工具0.2.3归档、公共Node SDK 0.1.15归档及TypeScript 5.9.3；将审阅过的两个本地归档放入vendor，禁用安装hooks后执行npm ci，再运行npm test与npm run build。构建脚本不读取宿主源码树；维护工具归档不是已正式发布的公共npm包，禁止改成浮动npx下载。vendor、node_modules和生成bundle均不提交。SDK NOTICE随候选保留，工具、编译器、运行时及官方CLI各自保持独立许可与分发边界。

默认合成验证fixture为64×48、1秒MP4（请求参数为5秒，实际字节与上报事实保持一致，不假装请求规格等于结果）。它通过真实隔离Node、SDK协商、Host Services和持久PG验证，不请求真实CLI。图片fixture另存`fixtures/verification-image.json`供显式后续选择；未因打包存在就计为再次执行。视频参数、不可变Draft成片、原命令与重启恢复另由渠道合成用例验收，真实视频与清晰度仍未验证，见[#266报告](../../../docs/evidence/plugin-jimeng-video-draft-266.md)。

组图/拆层按声明的最大输出数接受画布策略、Agent/MCP预算和原任务约束，100只是平台总上限，不能绕过当前画布的默认5张限制。Seedream 5.5可显式减少max_images；固定17张上限的Pro/Flash拆层在该画布策略下会在提交前拒绝。普通图片与样片/成片路径不受此限制影响。CLI参数仅支持驱动明确声明的子集，旧模型的其他参数完整保留并使用官方API，避免悄悄丢参。
