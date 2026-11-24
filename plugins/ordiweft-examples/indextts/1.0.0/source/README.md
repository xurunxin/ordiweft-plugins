# 独立 IndexTTS 适配候选

原创Apache-2.0适配器，仅依赖公开`pkg/pluginhost`，不导入Ordiweft internal或Morpheus，不持有网络客户端、凭据或业务数据库。稳定provider/model/node ID和原模型参数定义沿用内置IndexTTS；包ID为`ordiweft-examples/indextts`，与provider ID分开。

宿主负责受信连接、原素材用途/SHA、固定请求映射、单次提交、原回执等待、实际音频识别和原结果采用。独立程序检查服务/模型身份、情绪模式与互斥参考用途、上传回执、原job ID、结果路径/后端/大小/SHA，并只选择宿主暂存的私有音频句柄。合成test后端不获准用于语音；协议模拟器验证不能宣称真实模型或音质通过。

`policies/generate-*.json`是五种情绪模式的声明式JSON映射，只从原参数、正文和已保存上传回执取值；隐藏字段按内置实现的规则省略。`connection.profile.json`包含固定操作和实际JSON策略摘要，只引用服务端ITT_URL/ITT_API_KEY，不包含端点或Key。部署方需先绑定五份JSON策略，再独立审查和绑定连接；默认不授权HTTP，内网HTTP由管理员在自己的部署副本显式授权。文件不是连接权限。上传按固定speaker/emotion_audio唯一用途选择，向量顺序固定为happy、angry、sad、afraid、disgusted、melancholic、surprised、calm。

开发候选已在独立Linux宿主实际运行隔离协议和五种模式的标准生成队列，保留原用途、票据、参数与真实PCM文件；五份请求映射与内置真实HTTP提交逐模式对照。上述服务是隔离协议模拟器，真实GPU/模型/音质尚未验收。包含profile的固定发行闭包与干净消费者随后单独记录，不能用合成通过替代真实推理。

受限离线构建和静态打包使用现有工具：

```powershell
scripts/export-plugin-go-sdk.ps1 -Out <仓外新SDK目录>
node scripts/build-plugin-go-example.mjs --example indextts --sdk <SDK绝对路径> --sdk-commit <SDK固定提交> --out <仓外新输出目录>
ordiweft-plugin verify -root <输出目录>/bundle
ordiweft-plugin pack -root <输出目录>/bundle -out <仓外新ZIP>
```

这是独立适配候选，原票据只读恢复、在途更新及内置ID显式所有权迁移仍须完成后才可替换旧实现。现有安装器继续拒绝社区包直接占用内置indextts ID；不要改名或放宽占用保护绕过迁移。当前宿主原本地完整产物恢复不等于远端未完成任务的完整恢复。profile的单次请求30秒、每秒观察及15分钟整体窗口是显式部署上限，超过窗口保留原票据，不授予再次提交。

服务和GPU/模型/音色/私网部署独立维护，模型与素材许可不包含在软件Apache-2.0内。本任务不部署推理服务、替换现有插件或重提原任务。
