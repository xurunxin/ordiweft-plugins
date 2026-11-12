package pluginhost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

type JSONPolicyReference struct {
	Operation string `json:"operation"`
	Digest    string `json:"digest"`
}

func JSONPolicySetDigest(references []JSONPolicyReference) (string, error) {
	if len(references) > 64 {
		return "", ErrContract
	}
	ordered := append([]JSONPolicyReference{}, references...)
	slices.SortFunc(ordered, func(a, b JSONPolicyReference) int { return strings.Compare(a.Operation, b.Operation) })
	for i, r := range ordered {
		if !aigc.IDPattern.MatchString(r.Operation) || !ValidDigest(r.Digest) || i > 0 && ordered[i-1].Operation == r.Operation {
			return "", ErrContract
		}
	}
	body, _ := json.Marshal(ordered)
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:]), nil
}
