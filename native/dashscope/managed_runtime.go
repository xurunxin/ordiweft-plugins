package dashscope

import (
	"context"
	"errors"
	"net/http"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

// ManagedRuntime lets an isolated executable use a host-owned provider
// transport and account-scoped voice cache without transmitting credentials.
type ManagedRuntime struct {
	Transport     http.RoundTripper
	UploadAudio   func(context.Context, string, []byte) (string, error)
	LookupClone   func(context.Context, string) (*aigc.Voice, error)
	RememberClone func(context.Context, string, aigc.Voice) error
}

func NewManagedRuntime(region string, services ManagedRuntime) (*Plugin, error) {
	if !regions[region] || services.Transport == nil || services.UploadAudio == nil ||
		services.LookupClone == nil || services.RememberClone == nil {
		return nil, errors.New("invalid managed DashScope runtime")
	}
	p := New()
	p.region = region
	p.Client.Transport = services.Transport
	p.managed = &services
	return p, nil
}

// UploadReference carries out the provider-issued temporary upload for the
// already admitted reference. It is called only by the credential custodian.
func (p *Plugin) UploadReference(ctx context.Context, secrets map[string]string, model string, data []byte) (string, error) {
	base, err := p.endpoint(secrets)
	if err != nil {
		return "", err
	}
	return p.uploadAudio(ctx, base, secrets["API_KEY"], model, data)
}
