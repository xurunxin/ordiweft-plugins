# Fish Audio

候选包 `ordiweft-migrations/fish-audio` 保留原 `fish-audio` 渠道、三个合成模型，以及声音设计与私人克隆模型。普通合成和零样本合成分开匹配原请求，同一任务只占用一次供应商提交。已绑定音色按原顺序编码。

使用 Ordiweft 同次发布的公开 Go SDK 和 `scripts/build-plugin-go-example.mjs --example fish-audio` 生成仓外候选目录。宿主需声明 `connections.fish-audio-tts`、`connections.repeated-form-fields`、原音色绑定和输出音色身份。原服务端 API Key 或兼容共享连接与模板中的请求策略由管理员明确绑定。

声音设计保留候选顺序。私人克隆先保存原 ID 与状态，再用首段原样本作为预览。测试夹具使用静音 WAV，只验证合成路径，其他模式需在隔离服务中验证后才能切换实际渠道。
