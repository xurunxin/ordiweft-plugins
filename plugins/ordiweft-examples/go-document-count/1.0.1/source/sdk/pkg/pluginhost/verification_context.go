package pluginhost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// VerificationContextDigest binds test inputs; it is not an authority token or
// proof that tests ran. Only the host verifier may commit a ready transition.
func VerificationContextDigest(release, image, jsonSet, connectionSet string) (string, error) {
	if !ValidDigest(release) || !strings.HasPrefix(image, "sha256:") || !ValidDigest(strings.TrimPrefix(image, "sha256:")) || !ValidDigest(jsonSet) || !ValidDigest(connectionSet) {
		return "", ErrContract
	}
	material, _ := json.Marshal([]string{"node-host-services-verifier-v1", release, image, jsonSet, connectionSet})
	h := sha256.Sum256(material)
	return hex.EncodeToString(h[:]), nil
}
