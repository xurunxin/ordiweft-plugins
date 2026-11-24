package dashscope

import "gitea.nkit.top/xurx/Ordiweft/pkg/aigc"

// The channel owns region and model rules; the host supplies verified public
// rates and freezes the resulting tariff before admission. Missing rates stay unknown.
func modelPricing(model, region string) *aigc.PricingSpec {
	p := &aigc.PricingSpec{Provider: "dashscope", Model: model, Currency: "CNY", SourceURL: "https://help.aliyun.com/zh/model-studio/model-pricing", CheckedAt: "1970-01-01"}
	component := func(unit, per, dimension string) aigc.PricingComponent {
		return aigc.PricingComponent{Unit: unit, Per: per, CatalogPer: per, Dimension: dimension}
	}
	rule := func(when map[string]string, components ...aigc.PricingComponent) {
		p.Rules = append(p.Rules, aigc.PricingRule{When: when, Components: components})
	}
	switch model {
	case "qwen-image-2.1-pro", "qwen-image-2.1-turbo":
		rule(nil, component("image", "1", region))
	case "wan2.7-image-pro", "wan2.7-image":
		// In sequential mode n is an upper bound rather than an exact count.
		rule(map[string]string{"enable_sequential": "false"}, component("image", "1", region))
	case "qwen-image-3.0-pro", "qwen-image-3.0":
		for _, size := range []struct{ value, dimension string }{{"1024*1024", "1k"}, {"2048*2048", "2k"}} {
			rule(map[string]string{"size": size.value}, component("image", "1", region+"/"+size.dimension), component("input_image", "1", region+"/"+size.dimension))
		}
	case "wan3.0-video", "wan3.0-video-prime":
		for _, resolution := range []string{"480P", "720P", "1080P"} {
			dimension := region + "/" + map[string]string{"480P": "480p", "720P": "720p", "1080P": "1080p"}[resolution]
			rule(map[string]string{"resolution": resolution, "reference_video_count": "0"}, component("video_second", "1", dimension))
		}
	case "qwen3-tts-flash", "qwen3-tts-vd-2026-01-26", "qwen3-tts-vc-2026-01-22", "qwen-audio-3.0-tts-plus":
		rule(nil, component("character", "10000", region))
	case "qwen-audio-3.1-tts-flash":
		rule(nil, component("input_token", "1000000", region), component("output_token", "1000000", region))
	default:
		return nil
	}
	return p
}
