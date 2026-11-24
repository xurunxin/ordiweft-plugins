// Package pluginhost defines portable release and host-service contracts.
// It does not install code, authorize calls, resolve credentials, or submit work.
package pluginhost

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

const Version = 1
const MaxDescriptorBytes = 1 << 20

var ErrContract = errors.New("plugin_contract_invalid")
var packageID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}/[a-z][a-z0-9-]{0,62}$`)
var releaseVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[a-zA-Z0-9.-]+)?$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type Contribution struct {
	ID   string `json:"id"`
	Kind string `json:"kind"` // provider, content, node, processor, worker
}

// Requirement requests a host service. Only a trusted installation can grant it.
type Requirement struct {
	Service    string   `json:"service"`
	Version    int      `json:"version"`
	Operations []string `json:"operations"`
}

type Descriptor struct {
	FormatVersion int               `json:"formatVersion"`
	PackageID     string            `json:"packageId"`
	Version       string            `json:"version"`
	License       string            `json:"license"`
	Runtime       string            `json:"runtime"` // data, process, node, worker
	HostProtocol  int               `json:"hostProtocol"`
	Contributions []Contribution    `json:"contributions"`
	Requirements  []Requirement     `json:"requirements"`
	Files         []File            `json:"files"`
	Generation    *aigc.Manifest    `json:"generation,omitempty"`
	Definitions   map[string]string `json:"definitions,omitempty"`
}

var operations = map[string][]string{
	"connections": {"request"}, "assets": {"describe", "read", "decode", "release"},
	"transfers": {"upload", "download"}, "jobs": {"checkpoint", "query", "progress", "wait"},
	"outputs": {"begin", "write", "stage"}, "diagnostics": {"report"}, "usage": {"report"},
	"media": {"inspect", "derive"}, "state": {"get", "compareAndSet"},
	"catalog": {"list", "search"},
}

func ValidDigest(value string) bool    { return digestPattern.MatchString(value) }
func ValidPackageID(value string) bool { return packageID.MatchString(value) }
func ValidVersion(value string) bool   { return releaseVersion.MatchString(value) }

func ValidPath(value string) bool {
	valid := value != "" && !strings.EqualFold(value, "release.json") && len(value) <= 240 && path.Clean(value) == value &&
		!path.IsAbs(value) && value != "." && value != ".." && !strings.HasPrefix(value, "../") &&
		!strings.ContainsAny(value, "\\:\x00\r\n<>\"|?*") && !strings.HasSuffix(value, "/")
	if !valid {
		return false
	}
	for _, ch := range value {
		if ch < 32 || ch == 127 {
			return false
		}
	}
	for _, part := range strings.Split(value, "/") {
		if strings.TrimRight(part, " .") != part || part == "" {
			return false
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" ||
			(strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && slices.Contains([]string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³"}, strings.TrimPrefix(strings.TrimPrefix(base, "COM"), "LPT")) {
			return false
		}
	}
	return true
}

func Decode(body []byte) (Descriptor, error) {
	var d Descriptor
	if len(body) == 0 || len(body) > MaxDescriptorBytes {
		return d, ErrContract
	}
	if _, err := strictJSON(body); err != nil {
		return d, err
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return d, ErrContract
	}
	if dec.Decode(new(any)) != io.EOF {
		return d, ErrContract
	}
	return d, d.Validate()
}

func (d Descriptor) Validate() error {
	if d.FormatVersion != Version || d.HostProtocol != Version || !ValidPackageID(d.PackageID) ||
		!ValidVersion(d.Version) || d.License == "" || len(d.License) > 128 ||
		strings.ContainsAny(d.License, "\x00\r\n") || !slices.Contains([]string{"data", "process", "node", "worker"}, d.Runtime) {
		return ErrContract
	}
	if len(d.Contributions) == 0 || len(d.Contributions) > 64 || len(d.Files) == 0 || len(d.Files) > 4096 || len(d.Requirements) > len(operations) {
		return ErrContract
	}
	seen := map[string]bool{}
	providers := 0
	for _, c := range d.Contributions {
		if !aigc.IDPattern.MatchString(c.ID) || seen[c.ID] || !slices.Contains([]string{"provider", "content", "node", "processor", "worker"}, c.Kind) {
			return ErrContract
		}
		seen[c.ID] = true
		if c.Kind == "provider" {
			providers++
			if d.Generation == nil || c.ID != d.Generation.ID {
				return ErrContract
			}
		}
		if d.Runtime == "data" && c.Kind != "content" && c.Kind != "node" {
			return ErrContract
		}
	}
	if d.Generation != nil {
		if providers != 1 || d.Generation.Validate() != nil {
			return ErrContract
		}
	} else if providers != 0 {
		return ErrContract
	}
	seen = map[string]bool{}
	var total int64
	for _, f := range d.Files {
		// Case-insensitive uniqueness also protects Windows extraction.
		key := strings.ToLower(f.Path)
		if !ValidPath(f.Path) || seen[key] || !ValidDigest(f.SHA256) || f.Bytes < 0 || f.Bytes > 256<<20 {
			return ErrContract
		}
		seen[key] = true
		total += f.Bytes
		if total > 512<<20 {
			return ErrContract
		}
	}
	definitionFiles := map[string]bool{}
	for id, name := range d.Definitions {
		found := false
		for _, contribution := range d.Contributions {
			if contribution.ID == id && contribution.Kind != "provider" {
				found = true
			}
		}
		if !found || !ValidPath(name) || definitionFiles[name] {
			return ErrContract
		}
		definitionFiles[name] = true
		listed := false
		for _, file := range d.Files {
			if file.Path == name && file.Bytes > 0 && file.Bytes <= 64<<10 {
				listed = true
			}
		}
		if !listed {
			return ErrContract
		}
	}
	seen = map[string]bool{}
	for _, r := range d.Requirements {
		allowed, ok := operations[r.Service]
		if !ok || seen[r.Service] || r.Version != Version || len(r.Operations) == 0 {
			return ErrContract
		}
		seen[r.Service] = true
		used := map[string]bool{}
		for _, operation := range r.Operations {
			if used[operation] || !slices.Contains(allowed, operation) {
				return ErrContract
			}
			used[operation] = true
		}
	}
	if d.Runtime == "data" && len(d.Requirements) > 0 {
		return ErrContract
	}
	return nil
}

// Digest binds the descriptor and complete file inventory, independent of list order.
func (d Descriptor) Digest() (string, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	// A private copy prevents canonicalization from mutating the caller's slices.
	b, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("%w: descriptor encoding", ErrContract)
	}
	var normalized Descriptor
	if err = json.Unmarshal(b, &normalized); err != nil {
		return "", ErrContract
	}
	slices.SortFunc(normalized.Files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	slices.SortFunc(normalized.Contributions, func(a, b Contribution) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(normalized.Requirements, func(a, b Requirement) int { return strings.Compare(a.Service, b.Service) })
	for i := range normalized.Requirements {
		slices.Sort(normalized.Requirements[i].Operations)
	}
	b, err = json.Marshal(normalized)
	if err != nil {
		return "", ErrContract
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

// Binding is assigned by the host after admission; it never confers current authority.
type Binding struct {
	TaskID                string `json:"taskId"`
	Provider              string `json:"provider"`
	ReleaseDigest         string `json:"releaseDigest"`
	CatalogRevision       string `json:"catalogRevision"`
	ConfigurationRevision string `json:"configurationRevision"`
}

type Failure struct {
	Code       string `json:"code"`
	Stage      string `json:"stage"`
	Submission string `json:"submission"` // not_sent, sent, unknown; host evidence required
	RequestID  string `json:"requestId,omitempty"`
}

type Meter struct {
	Name     string `json:"name"`
	Unit     string `json:"unit"`
	Quantity string `json:"quantity"` // exact nonnegative decimal; not a price
	Source   string `json:"source"`   // provider_reported, estimated, confirmed
}
