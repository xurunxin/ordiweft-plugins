# SeedAudio 公开SDK迁移候选

Apache-2.0。稳定provider doubao-audio，包ordiweft-examples/doubao-audio，仅依赖公开Go SDK和标准库。模型/参数/纯文本输入与现有内置SeedAudio1.0一致；不会借迁移新增参考音频、图片或音色能力。开发候选已实际验证独立沙箱、普通PCM、显式WAV派生、HTTPS下载与错误回执；固定发行、完整应用、容量和整项迁移验收分别记录，不据此自动切换现有渠道。

宿主绑定受审JSON、官方连接和X-Api-Key；请求追踪头使用原任务ID，按官方允许的业务TraceID语义固定。1.0.1通过显式jobs.context取得原业务TraceID，并先checkpoint保存，再执行一次原提交；响应audio通过受限JSON-base64解码；URL仅由宿主从原响应/url选择并走公共HTTPS下载。适配器没有HTTP客户端、Key、路径或业务库；WAV显式调用公共audio.normalize-wav并保留来源，MP3直接交付实际字节校验；实际音频格式、时长、来源和采用由宿主验证。失败保留原回执，不自动重提。

1.0.2使用公开SDK的96MiB编码响应读取/解码上限，URL下载及解码音频仍最多64MiB；沙箱只记录audio是否存在，不另复制完整base64字符串。最大容量实际验收仍须单独完成，不能仅凭上限声明切换现有渠道。1.0.1的原TraceID checkpoint与成功/错误回执、固定1.0.0↔1.0.1的在途更新/回退已通过实际沙箱验证。配置/模型开关保持原值，安装需原生ID明确授权、独立验证与激活。

协议依据：[火山引擎音频生成](https://docs.volcengine.com/docs/doubao-voice/audio-generation-http?lang=zh)。官方支持的完整能力不等于本宿主当前已实现能力，模型/账号许可另行核对。本目录不携带服务凭据。
