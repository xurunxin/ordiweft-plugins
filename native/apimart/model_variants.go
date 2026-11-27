// SPDX-License-Identifier: Apache-2.0
package main

// The labels describe APIMart execution routes. An Ext alias may resolve to
// an upstream ID with no suffix. Each route keeps the same execution ID.
var modelVariants = map[string]struct{ label, note string }{
	"gemini-nano-banana-2.1":                  {"Nano banana 2.1 · 官方版", "1K、2K 或 4K，一次可生成 1–4 张，按实际 Token 计费。"},
	"gemini-nano-banana-2.1-ext":              {"Nano banana 2.1 · Ext", "同样支持 1K、2K 和 4K。一次一张，按分辨率计费，可开启官方渠道兜底。"},
	"gemini-2.5-flash-image-preview":          {"Nano banana · Ext", "一次一张，可开启官方渠道兜底。"},
	"gemini-3-pro-image-preview":              {"Nano banana Pro · Ext", "1K、2K 或 4K，一次一张，可开启官方渠道兜底。"},
	"gemini-3.1-flash-image-preview":          {"Nano banana 2 · Ext", "支持 0.5K 至 4K 和 Google 搜索，一次一张，可开启官方渠道兜底。"},
	"gemini-2.5-flash-image-preview-official": {"Nano banana · 官方版", "可一次生成 1–4 张。"},
	"gemini-3-pro-image-preview-official":     {"Nano banana Pro · 官方版", "1K、2K 或 4K，可一次生成 1–4 张。"},
	"gemini-3.1-flash-image-preview-official": {"Nano banana 2 · 官方版", "支持 0.5K 至 4K 和 Google 搜索，可一次生成 1–4 张。"},
	"gemini-3.1-flash-lite-image":             {"Nano banana 2 Lite", "固定 1K，最多 14 张参考图。Lite 与 Lite Ext 的参数相同，按各自渠道计费。"},
	"gemini-3.1-flash-lite-image-ext":         {"Nano banana 2 Lite · Ext", "固定 1K，最多 14 张参考图。与普通 Lite 参数相同，按 Ext 渠道计费。"},
	"gpt-image-2":                             {"GPT Image 2 · Ext", "一次一张，最多 15 张参考图，可开启官方兜底。官方版提供透明 PNG 蒙版和多图输出。"},
	"gpt-image-2-official":                    {"GPT Image 2 · 官方版", "最多 16 张参考图，支持透明 PNG 蒙版局部重绘，一次最多 4 张。"},
	"gpt-image-2.5-ext":                       {"GPT Image 2.5 · Ext", "通过「版本」选 Flare 或 Sunburst，按版本、分辨率和出图张数计费，支持 1–4 张。需要蒙版或精确像素尺寸时选择对应的 Flare／Sunburst 模型。"},
	"gpt-image-2.6-ext":                       {"GPT Image 2.6 · Ext", "通过「版本」选 Flare 或 Sunburst，一次一张，按张计费，支持 SVG 和 GIF。需要蒙版或标注层时选择对应的 Flare／Sunburst 模型。"},
	"muse-image-ext":                          {"Muse Image · Ext", "文生图或最多 9 张参考图，每次输出一张 WebP。"},
	"muse-video-ext":                          {"Muse Video · Ext", "文生或最多 9 张参考图，输出一段 2–10 秒、720p 的有声 MP4。"},
	"grok-imagine-2.0-ext":                    {"Grok Imagine 2.0 Image · Ext", "文生图最多 12 张。带参考图时最多用 5 张、输出 1 张，分辨率由接口决定。符合条件的单图可从原任务进入选区编辑。"},
	"grok-imagine-image-2.0":                  {"Grok Imagine 2.0 Image · 官方版", "最多 3 张参考图，支持 1K／2K，每次生成 1–10 张。"},
	"grok-imagine-1.5-video-ext":              {"Grok Imagine 1.5 Video · Ext", "支持首帧、首尾帧及最多 14 张参考图。1080p 仅用于文生或单首帧，视渠道开放情况而定。"},
	"grok-imagine-video-1.5":                  {"Grok Imagine 1.5 Video · 官方版", "支持文生及参考图生成，最多 7 张参考图，最高 1080p。"},
	"veo3.1-fast":                             {"Veo 3.1 Fast · Ext", "固定 8 秒，可用参考图或首尾帧。保留 Ext 渠道的原任务可续写至 15 秒。需要续写时请关闭官方兜底。"},
	"veo3.1-quality":                          {"Veo 3.1 Quality · Ext", "固定 8 秒，支持文生或首尾帧。保留 Ext 渠道的原任务可续写至 15 秒。需要续写时请关闭官方兜底。"},
	"veo3.1-lite":                             {"Veo 3.1 Lite", "固定 8 秒，仅支持文生视频。"},
	"veo3.1-fast-official":                    {"Veo 3.1 Fast · 官方版", "4、6 或 8 秒，支持种子、人物策略和最多 4 个输出样本。"},
	"veo3.1-quality-official":                 {"Veo 3.1 Quality · 官方版", "4、6 或 8 秒，支持种子、人物策略和最多 4 个输出样本。"},
	"gemini-omni-1.1-flash-ext":               {"Gemini Omni 1.1 Flash · Ext", "4、6、8 或 10 秒。参考模式可用 1 或 3 张图及最多 1 段视频。双帧、编辑和续写请选择官方版。"},
	"gemini-omni-1.1-flash":                   {"Gemini Omni 1.1 Flash · 官方版", "支持多主体参考、首尾帧、视频编辑和续写。"},
	"gemini-omni-1.2-flash-ext":               {"Gemini Omni 1.2 Flash · Ext", "支持系统指令和负面提示词，首尾帧模式除外。以视频素材编辑或续写时可另加最多 10 张参考图。"},
	"gemini-omni-1.2-flash":                   {"Gemini Omni 1.2 Flash", "支持首尾帧、多主体参考、视频编辑和续写，最高 4K。"},
}
