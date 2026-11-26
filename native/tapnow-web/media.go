package tapnowweb

import (
	"errors"
	"fmt"
	"gitea.nkit.top/xurx/Ordiweft/internal/assets"
)

type referenceDurations struct {
	video, audio float64
}

func (d *referenceDurations) add(mode Mode, kind string, seconds float64) error {
	var total *float64
	var limit *Range
	var label string
	switch kind {
	case "video":
		total, limit, label = &d.video, mode.VideoDuration, "视频"
	case "audio":
		total, limit, label = &d.audio, mode.AudioDuration, "音频"
	default:
		return nil
	}
	if seconds <= 0 || limit != nil && (seconds < limit.Min || (limit.Max > 0 && seconds > limit.Max+limit.Tolerance)) {
		return fmt.Errorf("参考%s时长越界", label)
	}
	*total += seconds
	return nil
}

func (d referenceDurations) validateTotals(mode Mode) error {
	outside := func(seconds float64, limit *Range) bool {
		return seconds > 0 && limit != nil && (seconds < limit.TotalMin || (limit.TotalMax > 0 && seconds > limit.TotalMax))
	}
	if outside(d.video, mode.VideoDuration) {
		return errors.New("参考视频总时长越界")
	}
	if outside(d.audio, mode.AudioDuration) {
		return errors.New("参考音频总时长越界")
	}
	return nil
}

func (d referenceDurations) estimateDuration(mode Mode) float64 {
	if mode.ID == "digital_human" {
		return d.audio
	}
	return d.video
}

func inspectMaterial(m Material, kind string) (Material, error) {
	if kind == "audio" {
		d, e := assets.WAVDuration(m.Bytes)
		if e != nil {
			return m, e
		}
		m.Duration = d
		m.Mime = "audio/wav"
		return m, nil
	}
	if kind == "video" {
		if _, _, e := assets.MP4Info(m.Bytes); e != nil {
			return m, e
		}
		duration, e := assets.MP4Duration(m.Bytes)
		if e != nil {
			return m, e
		}
		m.Mime = "video/mp4"
		m.Duration = duration
		return m, nil
	}
	format, ok := assets.Sniff(m.Bytes)
	if !ok || (format.Mime != "image/png" && format.Mime != "image/jpeg" && format.Mime != "image/webp") {
		return m, errors.New("Web 图片引用只支持真实 PNG/JPEG/WebP")
	}
	w, h, ok := assets.Dimensions(m.Bytes, format.Mime)
	if !ok || w <= 0 || h <= 0 {
		return m, errors.New("图片引用缺少真实像素")
	}
	m.Mime = format.Mime
	return m, nil
}
