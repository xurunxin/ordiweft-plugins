# Breeze TTS

本目录维护在线 API 与本地推理的 Go stdio 适配器。输入、语音 PCM 校验、音色目录和异步回执使用 Ordiweft 公共 Go SDK。发行包附 Linux/amd64 与 Windows/amd64 已构建适配器。本地模型权重需依照原许可另行部署。

使用由固定 Ordiweft 提交导出的公共 Go SDK，在仓库根运行 `pwsh -NoProfile -File scripts/build-breeze-tts.ps1 -SDK <导出目录> -SDKCommit <固定提交> -OutDir <全新外部目录>`。构建使用离线 Go 工具链，先对适配器运行合成测试，再输出两平台适配器及校验记录。
