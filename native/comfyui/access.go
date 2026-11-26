package comfyui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

// Access is stored with a workflow revision. It contains a credential
// reference only. Imported graphs and canvas jobs never supply its value.
type Access struct {
	Mode          string `json:"mode,omitempty"`
	CredentialEnv string `json:"credentialEnv,omitempty"`
}

func (a Access) validate(base string) error {
	switch a.Mode {
	case "", "direct":
		if a.CredentialEnv == "" {
			return nil
		}
	case "bearer", "api-key":
		if strings.HasPrefix(base, "https://") && strings.HasPrefix(a.CredentialEnv, "COMFYUI_") &&
			aigc.EnvPattern.MatchString(a.CredentialEnv) {
			return nil
		}
	}
	return fmt.Errorf("认证接入需使用 HTTPS 与 COMFYUI_ 开头的服务端凭据引用")
}

func (a Access) headers(base string) (http.Header, error) {
	if err := a.validate(base); err != nil {
		return nil, err
	}
	if a.Mode == "" || a.Mode == "direct" {
		return nil, nil
	}
	value := os.Getenv(a.CredentialEnv)
	if value == "" || strings.ContainsAny(value, "\r\n\x00") {
		return nil, fmt.Errorf("ComfyUI 接入凭据未配置")
	}
	header := make(http.Header)
	if a.Mode == "bearer" {
		header.Set("Authorization", "Bearer "+value)
	} else {
		header.Set("X-API-Key", value)
	}
	return header, nil
}

// Import inspects only the selected server's schema, then freezes the graph,
// endpoint and access mode as an immutable workflow definition.
func Import(ctx context.Context, name, base string, raw json.RawMessage, access Access) (Definition, error) {
	base, err := Endpoint(base)
	if err != nil {
		return Definition{}, err
	}
	if access.Mode == "direct" && access.CredentialEnv == "" {
		access = Access{}
	}
	if err = access.validate(base); err != nil {
		return Definition{}, err
	}
	info, err := inspect(ctx, base, raw, access)
	if err != nil {
		return Definition{}, err
	}
	d, err := Compile(name, base, raw, info)
	if err == nil && access.Mode != "" {
		d.Access = &access
		data, _ := json.Marshal(d)
		digest := sha256.Sum256(data)
		d.Manifest.ID = "comfy-" + hex.EncodeToString(digest[:10])
	}
	return d, err
}
