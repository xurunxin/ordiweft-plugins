# 豆包语音合成

独立候选沿用稳定 provider `doubao-speech`、模型 `seed-tts-2.0` 和语音节点。插件经宿主的原任务流式接口执行，API Key 留在宿主，受审策略固定官方地址、资源 ID、唯一提交槽和原任务请求标识。支持命名 API Key 连接，也保留部署已有环境变量绑定。已有官方 2.0 音色连线、文本、语音指令、语速、音量及采样率沿用原设置。

候选累积一次原始流，收到结束帧并核对原响应完成后才暂存音频。断线、供应商错误、缺失结束帧均不产生部分输出。实际格式、时长和费用归属由宿主核验。

运行仓库的 `build-plugin-go-example.mjs --example doubao-speech`，使用固定公开 SDK 和仓外目录。CI 或管理员可核对 `fixtures/verification.json` 中的两段合成 PCM 与结束帧。宿主需提供 `connections.voice-request-fields` v1。安装审核需绑定本目录的 JSON 映射与连接策略，执行候选合成验证。兼容迁移和原生回退使用现有原生授权、活动版本和原回执接口。候选现用于合成验证。
