package aigc

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"regexp"
	"time"
)

// PricingSpec is trusted model metadata, never supplied by generation requests.
// Currency conversion, policy multipliers and settlement remain host duties.
type PricingSpec struct {
	Provider  string        `json:"provider"`
	Model     string        `json:"model"`
	Currency  string        `json:"currency,omitempty"`
	SourceURL string        `json:"sourceUrl,omitempty"`
	CheckedAt string        `json:"checkedAt,omitempty"`
	Rules     []PricingRule `json:"rules,omitempty"`
}
type PricingRule struct {
	When       map[string]string  `json:"when,omitempty"`
	Components []PricingComponent `json:"components"`
}
type PricingComponent struct {
	Unit       string `json:"unit"`
	Rate       string `json:"rate"`
	Per        string `json:"per"`
	Dimension  string `json:"dimension,omitempty"`
	CatalogPer string `json:"catalogPer,omitempty"`
}

var pricingDecimal = regexp.MustCompile(`^(0|[1-9][0-9]{0,11})(\.[0-9]{1,9})?$`)

func validatePricing(model Model) error {
	p := model.Pricing
	if p == nil {
		return nil
	}
	bad := func() error { return fmt.Errorf("invalid pricing for model %s", model.ID) }
	if !IDPattern.MatchString(p.Provider) || p.Model == "" || len(p.Model) > 256 || len(p.Rules) > 128 {
		return bad()
	}
	if len(p.Rules) == 0 {
		return nil
	} // Canonical host pricing recipe.
	u, e := url.Parse(p.SourceURL)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || len(p.SourceURL) > 2048 {
		return bad()
	}
	if _, e = time.Parse("2006-01-02", p.CheckedAt); e != nil {
		return bad()
	}
	if p.Currency != "USD" && p.Currency != "CNY" {
		return bad()
	}
	seen := map[string]bool{}
	for _, rule := range p.Rules {
		if len(rule.When) > 16 || len(rule.Components) == 0 || len(rule.Components) > 16 {
			return bad()
		}
		for key, value := range rule.When {
			f, ok := model.Fields[key]
			if (!ok || f.Type == "object" || f.Type == "array") && key != "reference_image_count" && key != "reference_video_count" && key != "reference_audio_count" {
				return bad()
			}
			if ok && f.Type == "string" && len(f.Enum) == 0 {
				return bad()
			}
			if len(value) > 200 {
				return bad()
			}
		}
		raw, _ := json.Marshal(rule.When)
		key := string(raw)
		if seen[key] {
			return bad()
		}
		seen[key] = true
		for _, c := range rule.Components {
			switch c.Unit {
			case "job", "image", "input_image", "video_second", "audio_second", "input_token", "input_text_token", "input_image_token", "output_token", "uncached_input_token", "cache_read_token", "cache_write_token", "character", "utf8_byte", "billable_character":
			default:
				return bad()
			}
			if (!pricingDecimal.MatchString(c.Rate) && !(c.Rate == "" && c.CatalogPer != "")) || !pricingDecimal.MatchString(c.Per) || len(c.Dimension) > 128 {
				return bad()
			}
			per, _ := new(big.Rat).SetString(c.Per)
			if per.Sign() <= 0 {
				return bad()
			}
			if c.CatalogPer != "" {
				if !pricingDecimal.MatchString(c.CatalogPer) {
					return bad()
				}
				v, _ := new(big.Rat).SetString(c.CatalogPer)
				if v.Sign() <= 0 {
					return bad()
				}
			}
		}
	}
	return nil
}
