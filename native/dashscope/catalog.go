// Package dashscope adapts Alibaba Cloud Model Studio through the host AIGC SDK.
package dashscope

import "gitea.nkit.top/xurx/Ordiweft/pkg/aigc"

func enum(label, value string, values ...string) aigc.Field {
	f := aigc.Field{Type: "string", Label: label, Default: value, Enum: []any{}}
	for _, v := range values {
		f.Enum = append(f.Enum, v)
	}
	return f
}
func integer(label string, value, min, max float64, control string) aigc.Field {
	return aigc.Field{Type: "integer", Label: label, Default: value, Minimum: &min, Maximum: &max, Control: control}
}
func boolean(label string, value bool) aigc.Field {
	return aigc.Field{Type: "boolean", Label: label, Default: value}
}
func catalog() aigc.Manifest {
	m := aigc.Manifest{APIVersion: 2, ID: "dashscope", Name: "阿里云百炼", Version: "1.0.0", Runtime: "builtin", Auth: "api-key", Secrets: []string{"API_KEY", "WORKSPACE_ID"},
		Scheduling: aigc.Scheduling{Independent: true, Concurrency: aigc.RemoteHostConcurrency, MaxQueued: 100, Cancelable: false},
		Nodes: []aigc.Node{
			{ID: "image", Label: "百炼图片生成", Capability: "image.generate", Model: "qwen-image-2.1-turbo"},
			{ID: "video", Label: "百炼视频生成", Capability: "video.generate", Model: "wan3.0-video"},
			{ID: "speech", Label: "百炼语音合成", Capability: "speech.generate", Model: "qwen-audio-3.1-tts-flash"},
		}, Models: []aigc.Model{}}
	for _, id := range []string{"qwen-image-3.0-pro", "qwen-image-3.0", "qwen-image-2.1-pro", "qwen-image-2.1-turbo", "wan2.7-image-pro", "wan2.7-image"} {
		wan := id == "wan2.7-image-pro" || id == "wan2.7-image"
		limit := 3
		if id == "qwen-image-2.1-pro" || id == "qwen-image-2.1-turbo" {
			limit = 10
		}
		fields := map[string]aigc.Field{
			"size": enum("输出尺寸", "auto", "auto", "1024*1024", "1536*1024", "1024*1536", "2048*2048"),
			"n":    integer("图片数量", 1, 1, 6, "count"), "watermark": boolean("添加水印", false),
			"seed": {Type: "integer", Label: "随机种子（留空随机）", Minimum: ptr(0), Maximum: ptr(2147483647)},
		}
		maxPrompt := 20000
		if wan {
			limit, maxPrompt = 9, 5000
			fields["size"] = enum("输出分辨率", "2K", "1K", "2K")
			if id == "wan2.7-image-pro" {
				fields["size"] = enum("输出分辨率", "2K", "1K", "2K", "4K")
			}
			fields["enable_sequential"] = boolean("组图生成", false)
			fields["n"] = integer("图片数量上限", 1, 1, 12, "count")
			fields["thinking_mode"] = boolean("思考模式", true)
		} else {
			fields["prompt_extend"] = boolean("优化提示词", true)
			fields["prompt_extend_mode"] = enum("提示词优化方式", "direct", "direct", "agent")
			f := fields["prompt_extend_mode"]
			f.OptionLabels = map[string]string{"direct": "直接增强", "agent": "智能体增强（仅文生图）"}
			f.VisibleWhen = map[string]any{"prompt_extend": true}
			fields["prompt_extend_mode"] = f
			fields["enable_thinking"] = boolean("思考模式", true)
			f = fields["enable_thinking"]
			f.VisibleWhen = map[string]any{"prompt_extend": true}
			fields["enable_thinking"] = f
			if limit == 3 {
				fields["negative_prompt"] = aigc.Field{Type: "string", Label: "反向提示词", Control: "textarea", MaxLength: 500}
			}
		}
		m.Models = append(m.Models, aigc.Model{ID: id, Label: id, Capability: "image.generate", InputKinds: []string{"image"}, MaxInputs: limit, Fields: fields, Prompt: &aigc.PromptSpec{Label: "图像描述 / 编辑指令", Required: true, MaxLength: maxPrompt}})
	}
	for _, id := range []string{"wan3.0-video", "wan3.0-video-prime"} {
		mode := enum("参考图用法", "reference", "reference", "first_frame", "first_last_frame")
		mode.OptionLabels = map[string]string{"reference": "多图参考（可无图）", "first_frame": "首帧（1张图）", "first_last_frame": "首尾帧（2张图）"}
		mode.InputModes = map[string]aigc.InputMode{"reference": {Intent: "reference", Images: aigc.InputRange{Min: 0, Max: 10}}, "first_frame": {Intent: "first_frame", Images: aigc.InputRange{Min: 1, Max: 1}}, "first_last_frame": {Intent: "first_last_frame", Images: aigc.InputRange{Min: 2, Max: 2}}}
		m.Models = append(m.Models, aigc.Model{ID: id, Label: id, Capability: "video.generate", InputKinds: []string{"image"}, MaxInputs: 10, Prompt: &aigc.PromptSpec{Label: "视频描述", Required: true, MaxLength: 20000}, Fields: map[string]aigc.Field{
			"input_mode": mode, "resolution": enum("分辨率", "720P", "480P", "720P", "1080P"),
			"ratio":    enum("画面比例", "adaptive", "adaptive", "21:9", "16:9", "4:3", "1:1", "3:4", "9:16"),
			"duration": integer("时长（秒）", 5, 2, 30, "duration"), "audio": boolean("生成声音", true), "prompt_extend": boolean("优化提示词", true), "watermark": boolean("添加水印", false),
			"seed": {Type: "integer", Label: "随机种子（留空随机）", Minimum: ptr(0), Maximum: ptr(2147483647)},
		}})
	}
	m.Models = append(m.Models, aigc.Model{ID: "qwen3-tts-flash", Label: "Qwen3-TTS-Flash（系统音色）", Capability: "speech.generate", InputKinds: []string{}, Prompt: &aigc.PromptSpec{Label: "合成文本", Required: true, MaxLength: 600}, Fields: map[string]aigc.Field{
		"voice":         {Type: "string", Label: "系统音色 ID", Control: "voice", Default: "Cherry", Required: true, MaxLength: 128},
		"language_type": enum("语言", "Auto", "Auto", "Chinese", "English", "German", "Italian", "Portuguese", "Spanish", "Japanese", "Korean", "French", "Russian"),
	}})
	addVoiceModels(&m)
	for i := range m.Models {
		id := m.Models[i].ID
		v := &aigc.ModelQueueLimit{Bucket: id, Concurrency: aigc.RemoteHostConcurrency}
		switch id {
		case "qwen-image-3.0-pro":
			v.RPM = 5
			v.IntervalMS = 12000
		case "qwen-image-3.0", "qwen-image-2.1-pro":
			v.RPM = 20
			v.IntervalMS = 3000
		case "qwen-image-2.1-turbo":
			v.RPM = 120
			v.IntervalMS = 500
		case "wan2.7-image-pro", "wan2.7-image", "wan3.0-video", "wan3.0-video-prime":
			v.RPS = 5
			v.IntervalMS = 200
			v.Concurrency = 5
		case "qwen3-tts-flash", "qwen-audio-3.1-tts-flash", "qwen-voice-enrollment":
			v.RPM = 180
			v.IntervalMS = 334
		case "voice-enrollment", "voice-enrollment-clone":
			v.Bucket = "voice-enrollment"
			v.RPS = 10
			v.IntervalMS = 100
		default:
			continue // Account-specific or unpublished quota; do not invent a rate.
		}
		m.Models[i].QueueLimit = v
	}
	return m
}
func ptr(v float64) *float64 { return &v }
