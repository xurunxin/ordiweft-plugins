package pluginhost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var ErrIncompatible = errors.New("plugin_capability_incompatible")
var capabilityID = regexp.MustCompile(`^[a-z][a-z0-9.-]{0,95}$`)
var numericVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// SDKVersion identifies this private-channel SDK contract, separately from the Go module version.
const SDKVersion = "0.1.15"

func SDKCapabilities() []Capability {
	return []Capability{{ID: "invocation.negotiate", Version: 1}}
}

// Capability versions are exact wire/semantic revisions, not grants or semver ranges.
type Capability struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

type SDKRequirement struct {
	MinimumVersion string       `json:"minimumVersion"`
	Capabilities   []Capability `json:"capabilities"`
}

type ChannelRequirement struct {
	Version        int          `json:"version"`
	MinimumVersion string       `json:"minimumVersion"`
	Capabilities   []Capability `json:"capabilities"`
}

type Compatibility struct {
	Version int                 `json:"version"`
	Host    []Capability        `json:"host"`
	SDK     SDKRequirement      `json:"sdk"`
	Channel *ChannelRequirement `json:"channel,omitempty"`
}

func HostCapabilities() []Capability {
	// Image review is host-mediated and absent from the sandbox's generic tools.
	return []Capability{{"runtime.negotiation", 1}, {"generation.output-count", 1}, {"media.inspect.mp4", 1}, {"media.inspect.transparent", 1}, {"channel.negotiation", 1}, {"generation.quote", 1}, {"generation.image-review", 1}, {"generation.voice-bindings", 1}, {"outputs.voice-identity", 1}, {"connections.codex-profile", 1}, {"connections.submission-preflight", 1}, {"connections.video-reference-duration", 1}, {"generation.transcription", 1}, {"connections.voice-request-fields", 1}, {"connections.repeated-form-fields", 1}, {"connections.fish-audio-tts", 1}}
}

func validCapabilities(items []Capability) bool {
	if len(items) < 1 || len(items) > 32 {
		return false
	}
	seen := map[Capability]bool{}
	for _, item := range items {
		if !capabilityID.MatchString(item.ID) || item.Version < 1 || item.Version > 65535 || seen[item] {
			return false
		}
		seen[item] = true
	}
	return true
}

func SupportsCapabilities(available, required []Capability) bool {
	for _, capability := range required {
		if !slices.Contains(available, capability) {
			return false
		}
	}
	return true
}

func versionParts(value string) ([3]int, error) {
	var out [3]int
	if !numericVersion.MatchString(value) {
		return out, ErrContract
	}
	for i, part := range strings.Split(value, ".") {
		n, err := strconv.Atoi(part)
		if err != nil || n > 1000000 {
			return out, ErrContract
		}
		out[i] = n
	}
	return out, nil
}

func VersionAtLeast(value, minimum string) bool {
	a, err := versionParts(value)
	if err != nil {
		return false
	}
	b, err := versionParts(minimum)
	if err != nil {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return true
}

func (r ChannelRequirement) Validate() error {
	if r.Version != 1 || !VersionAtLeast(r.MinimumVersion, "0.0.0") || !validCapabilities(r.Capabilities) {
		return ErrContract
	}
	return nil
}

func (c Compatibility) Validate() error {
	if c.Version != 1 || !validCapabilities(c.Host) || !VersionAtLeast(c.SDK.MinimumVersion, "0.1.15") || !validCapabilities(c.SDK.Capabilities) || !slices.Contains(c.Host, Capability{"runtime.negotiation", 1}) || !slices.Contains(c.SDK.Capabilities, Capability{"invocation.negotiate", 1}) {
		return ErrContract
	}
	if c.Channel != nil && (c.Channel.Validate() != nil || !slices.Contains(c.Host, Capability{"channel.negotiation", 1})) {
		return ErrContract
	}
	return nil
}

func (d Descriptor) CheckHostCompatibility() error {
	if d.Compatibility == nil {
		return nil
	}
	if d.Compatibility.Validate() != nil || !SupportsCapabilities(HostCapabilities(), d.Compatibility.Host) {
		return ErrIncompatible
	}
	return nil
}

type HostOffer struct {
	Version  int           `json:"version"`
	Profile  string        `json:"profile"`
	Host     []Capability  `json:"host"`
	Required Compatibility `json:"required"`
}

func (d Descriptor) HostOffer() (*HostOffer, error) {
	if err := d.CheckHostCompatibility(); err != nil {
		return nil, err
	}
	if d.Compatibility == nil {
		return nil, nil
	}
	// Roundtrip freezes the descriptor-owned requirement slices for this invocation.
	body, _ := json.Marshal(d.Compatibility)
	var required Compatibility
	if json.Unmarshal(body, &required) != nil {
		return nil, ErrContract
	}
	o := &HostOffer{Version: 1, Host: HostCapabilities(), Required: required}
	o.Profile = profileDigest(o)
	return o, nil
}

func profileDigest(value any) string {
	b, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	v, err := strictJSON(b)
	if err != nil {
		return ""
	}
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	delete(m, "profile")
	b, err = json.Marshal(m)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (o HostOffer) Validate() error {
	if o.Version != 1 || !ValidDigest(o.Profile) || profileDigest(o) != o.Profile || !validCapabilities(o.Host) || o.Required.Validate() != nil || !SupportsCapabilities(o.Host, o.Required.Host) {
		return ErrContract
	}
	return nil
}

// AcceptSDK confirms compatibility only. Business services keep their own grants and fences.
func (o HostOffer) AcceptSDK(body []byte) error {
	var ack struct {
		Profile      string       `json:"profile"`
		SDKVersion   string       `json:"sdkVersion"`
		Capabilities []Capability `json:"capabilities"`
	}
	if decodeStrictObject(body, &ack) != nil || o.Validate() != nil || ack.Profile != o.Profile || !VersionAtLeast(ack.SDKVersion, o.Required.SDK.MinimumVersion) || !validCapabilities(ack.Capabilities) || !SupportsCapabilities(ack.Capabilities, o.Required.SDK.Capabilities) {
		return ErrIncompatible
	}
	return nil
}

func decodeStrictObject(body []byte, out any) error {
	if len(body) == 0 || len(body) > MaxControlBytes {
		return ErrContract
	}
	if _, err := strictJSON(body); err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil {
		return ErrContract
	}
	return nil
}

type ChannelAdvertisement struct {
	Version               int          `json:"version"`
	ImplementationVersion string       `json:"implementationVersion"`
	Profile               string       `json:"profile"`
	Capabilities          []Capability `json:"capabilities"`
}

func DecodeChannelAdvertisement(body []byte, required ChannelRequirement) (ChannelAdvertisement, error) {
	var out ChannelAdvertisement
	if len(body) > 64<<10 || decodeStrictObject(body, &out) != nil || required.Validate() != nil || out.Version != required.Version || !VersionAtLeast(out.ImplementationVersion, required.MinimumVersion) || !validCapabilities(out.Capabilities) || !SupportsCapabilities(out.Capabilities, required.Capabilities) || !ValidDigest(out.Profile) || profileDigest(out) != out.Profile {
		return out, ErrIncompatible
	}
	return out, nil
}

type ChannelNegotiation struct {
	Operation string             `json:"operation"`
	Required  ChannelRequirement `json:"required"`
}

func (p ConnectionPolicy) CheckCompatibility(d Descriptor) error {
	if p.CredentialProvider != "" {
		if d.Generation == nil || d.Generation.ID != "codex" || d.Compatibility == nil || !SupportsCapabilities(d.Compatibility.Host, []Capability{{"connections.codex-profile", 1}}) {
			return ErrIncompatible
		}
	}
	for _, operation := range p.Operations {
		if operation.FishAudioTTS != "" && (d.Generation == nil || d.Generation.ID != "fish-audio" || d.Compatibility == nil ||
			!SupportsCapabilities(d.Compatibility.Host, []Capability{{"connections.fish-audio-tts", 1}})) {
			return ErrIncompatible
		}
		if len(operation.RepeatedFormFields) != 0 && (d.Compatibility == nil ||
			!SupportsCapabilities(d.Compatibility.Host, []Capability{{"connections.repeated-form-fields", 1}})) {
			return ErrIncompatible
		}
	}
	if d.Compatibility == nil || d.Compatibility.Channel == nil {
		return nil
	}
	if p.Negotiation == nil || p.Negotiation.Required.Validate() != nil || !VersionAtLeast(p.Negotiation.Required.MinimumVersion, d.Compatibility.Channel.MinimumVersion) || !SupportsCapabilities(p.Negotiation.Required.Capabilities, d.Compatibility.Channel.Capabilities) {
		return ErrIncompatible
	}
	return nil
}

func ChannelProbeCallID(original string) string {
	h := sha256.Sum256([]byte("channel-negotiation/v1:" + original))
	return "negotiation_" + hex.EncodeToString(h[:])
}

// CompareStableRelease compares numeric components followed by stable status.
// Distribution uses it for stable release selection and stable dependency bounds.
func CompareStableRelease(left, right string) int {
	leftCore, _, leftPre := strings.Cut(left, "-")
	rightCore, _, rightPre := strings.Cut(right, "-")
	order := slices.CompareFunc(strings.Split(leftCore, "."), strings.Split(rightCore, "."), func(a, b string) int {
		a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
		if len(a) != len(b) {
			return len(a) - len(b)
		}
		return strings.Compare(a, b)
	})
	if order == 0 && leftPre != rightPre {
		if leftPre {
			return -1
		}
		return 1
	}
	return order
}
