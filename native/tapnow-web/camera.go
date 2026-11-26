package tapnowweb

import (
	_ "embed"
	"encoding/json"
	"errors"
	"slices"
)

//go:embed camera.json
var cameraJSON []byte

type CameraOption struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Image string `json:"image,omitempty"`
	Type  string `json:"type,omitempty"`
}
type CameraCatalog struct {
	Cameras   []CameraOption `json:"cameras"`
	Lenses    []CameraOption `json:"lenses"`
	Focals    []CameraOption `json:"focals"`
	Apertures []CameraOption `json:"apertures"`
	Models    []string       `json:"models"`
	Source    string         `json:"source"`
}
type CameraControl struct {
	Enabled     bool   `json:"enabled"`
	CameraKey   string `json:"cameraKey"`
	LensKey     string `json:"lensKey"`
	FocalKey    string `json:"focalKey"`
	ApertureKey string `json:"apertureKey"`
}

func CameraOptions() CameraCatalog { var c CameraCatalog; _ = json.Unmarshal(cameraJSON, &c); return c }
func validateCamera(r Request) error {
	if r.CameraControl == nil || !r.CameraControl.Enabled {
		return nil
	}
	c := CameraOptions()
	if r.Kind != "image" || !slices.Contains(c.Models, r.Model) {
		return errors.New("当前模型不支持 Lens")
	}
	cfg := r.CameraControl
	for key, options := range map[string][]CameraOption{cfg.CameraKey: c.Cameras, cfg.LensKey: c.Lenses, cfg.FocalKey: c.Focals, cfg.ApertureKey: c.Apertures} {
		if !slices.ContainsFunc(options, func(o CameraOption) bool { return o.Key == key }) {
			return errors.New("相机组合参数无效")
		}
	}
	return nil
}
func generationPath(r Request) string {
	if r.CameraControl != nil && r.CameraControl.Enabled {
		return "/api/bff/generation/image/camera_control"
	}
	return "/api/conversation/v1/generations/" + r.Kind
}
