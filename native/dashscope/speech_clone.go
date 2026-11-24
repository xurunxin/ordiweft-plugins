package dashscope

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

func (p *Plugin) qwenBase(base string) string {
	if p.baseURL != "" {
		return base
	}
	switch p.region {
	case "cn-beijing":
		return "https://dashscope.aliyuncs.com/api/v1"
	case "ap-southeast-1":
		return "https://dashscope-intl.aliyuncs.com/api/v1"
	}
	return base
}

// Cache identity by account, user, exact source bytes and target model. Creating a
// voice and synthesizing speech are each submitted once; failures never replay.
func (p *Plugin) cloneForSpeech(ctx context.Context, base string, g aigc.Generation, r aigc.Request, checkpoint func(aigc.Result)) (*aigc.Voice, error) {
	if g.Model != "qwen-audio-3.1-tts-flash" && g.Model != "qwen-audio-3.0-tts-plus" {
		return nil, errors.New("当前合成入口不支持音频自动复刻，请使用 Qwen-Audio 3.1 Flash 或 3.0 Plus")
	}
	if len(r.Inputs) != 1 || r.Inputs[0].Kind != "audio" {
		return nil, errors.New("请只接入一段音色参考音频")
	}
	sample := r.Inputs[0].Data
	if _, _, err := audioSample(sample); err != nil {
		return nil, err
	}
	target, operation := g.Model, "voice-enrollment-clone"
	source := sha256.Sum256(sample)
	digest := hex.EncodeToString(source[:])
	keySum := sha256.Sum256([]byte(r.Secrets["API_KEY"] + "\x00" + r.Secrets["WORKSPACE_ID"] + "\x00" + p.region + "\x00" + r.Identity.User + "\x00" + target + "\x00" + digest))
	key := hex.EncodeToString(keySum[:])
	p.voiceMu.Lock()
	defer p.voiceMu.Unlock()
	if p.voiceCache == nil {
		p.voiceCache = map[string]*aigc.Voice{}
	}
	voice := p.voiceCache[key]
	if voice == nil && p.managed != nil {
		var err error
		voice, err = p.managed.LookupClone(ctx, target)
		if err != nil {
			return nil, err
		}
	}
	if voice == nil && p.VoiceCacheDir != "" {
		b, err := os.ReadFile(filepath.Join(p.VoiceCacheDir, key+".json"))
		if err == nil {
			var v aigc.Voice
			if json.Unmarshal(b, &v) != nil || v.Provider != "dashscope" || v.Kind != "clone" || !voiceID.MatchString(v.ID) || len(v.CompatibleModels) != 1 || v.CompatibleModels[0] != target {
				return nil, errors.New("音色复刻缓存异常，请检查保留的音色记录")
			}
			voice = &v
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	notify := func(v *aigc.Voice) {
		if checkpoint != nil {
			b, _ := json.Marshal(map[string]any{"voiceId": v.ID, "targetModel": target, "sourceAudioSHA256": digest, "phase": "voice-ready"})
			checkpoint(aigc.Result{Metadata: b})
		}
	}
	if voice != nil {
		copy := *voice
		notify(&copy)
		return &copy, nil
	}
	params := map[string]any{"target_model": target, "prefix": "cv" + digest[:8], "preferred_name": "cv" + digest[:10]}
	out, err := p.createVoice(ctx, base, aigc.Generation{Model: operation, Capability: "voice.design"}, params, r, checkpoint)
	if err != nil {
		return nil, err
	}
	voice = out.Media[0].Voice
	voice.Label = "音频复刻音色"
	if p.VoiceCacheDir != "" {
		if err = os.MkdirAll(p.VoiceCacheDir, 0700); err != nil {
			return nil, err
		}
		b, _ := json.Marshal(voice)
		path := filepath.Join(p.VoiceCacheDir, key+".json")
		if err = os.WriteFile(path+".tmp", b, 0600); err != nil {
			return nil, err
		}
		if err = os.Rename(path+".tmp", path); err != nil {
			return nil, err
		}
	}
	if p.managed != nil {
		if err = p.managed.RememberClone(ctx, target, *voice); err != nil {
			return nil, err
		}
	}
	p.voiceCache[key] = voice
	notify(voice)
	copy := *voice
	return &copy, nil
}
