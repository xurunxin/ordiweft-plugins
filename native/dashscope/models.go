package dashscope

import (
	"context"
	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	"gitea.nkit.top/xurx/Ordiweft/pkg/providermodels"
)

type catalogPlugin struct {
	*Plugin
	manifest aigc.Manifest
}

func (p *catalogPlugin) Manifest() aigc.Manifest    { return p.manifest }
func (p *catalogPlugin) CatalogSource() aigc.Plugin { return p.Plugin }
func (p *catalogPlugin) Validate(ctx context.Context, r aigc.Request) error {
	if err := aigc.ValidateGeneration(p.manifest, r); err != nil {
		return err
	}
	return p.Plugin.Validate(ctx, r)
}
func (p *catalogPlugin) Execute(ctx context.Context, r aigc.Request, checkpoint func(aigc.Result)) (aigc.Result, error) {
	if err := p.Validate(ctx, r); err != nil {
		return aigc.Result{}, err
	}
	return p.Plugin.Execute(ctx, r, checkpoint)
}
func (p *Plugin) ListModels(ctx context.Context, secrets map[string]string) ([]string, error) {
	origin, err := p.endpoint(secrets)
	if err != nil {
		return nil, err
	}
	if p.baseURL == "" && p.region == "us-east-1" {
		origin = "https://dashscope-us.aliyuncs.com/api/v1"
	}
	return providermodels.List(ctx, p.Client, origin+"/models", secrets["API_KEY"], true)
}
func (p *Plugin) WithModelCatalog(c aigc.ModelCatalog) (aigc.Plugin, []string, error) {
	m, unknown, err := aigc.CatalogManifest(p.Manifest(), c)
	return &catalogPlugin{p, m}, unknown, err
}
