package dashscope

import (
	"context"
	"errors"
	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	"time"
)

func (p *Plugin) waitVoiceReady(ctx context.Context, base, key string, voice *aigc.Voice) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for {
		out, err := p.call(ctx, "POST", base, customizationPath, key, map[string]any{"model": "voice-enrollment", "input": map[string]any{"action": "query_voice", "voice_id": voice.ID}}, false)
		if err != nil {
			return err
		}
		if out.Output.TargetModel != "" && (len(voice.CompatibleModels) != 1 || out.Output.TargetModel != voice.CompatibleModels[0]) {
			return errors.New("供应商返回的音色目标模型不匹配")
		}
		switch out.Output.Status {
		case "OK":
			return nil
		case "DEPLOYING":
		case "UNDEPLOYED":
			return errors.New("音色未通过供应商审核，音色 ID 已保留")
		default:
			return errors.New("供应商未确认音色可用，音色 ID 已保留，请勿重复创建")
		}
		select {
		case <-ctx.Done():
			return errors.New("音色仍在处理中，音色 ID 已保留")
		case <-time.After(time.Second):
		}
	}
}
