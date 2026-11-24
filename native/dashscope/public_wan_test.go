package dashscope

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	contract "gitea.nkit.top/xurx/Ordiweft/pkg/pluginhost"
)

func TestPublicWanCandidatePreservesNativeModelRules(t *testing.T) {
	root := filepath.Join("..", "..", "examples", "plugin-go", "dashscope-wan")
	read := func(name string) []byte {
		t.Helper()
		b, e := os.ReadFile(filepath.Join(root, name))
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	var descriptor contract.Descriptor
	if err := json.Unmarshal(read("release.template.json"), &descriptor); err != nil || descriptor.Generation == nil {
		t.Fatal(err)
	}
	manifest := *descriptor.Generation
	original := map[string]aigc.Model{}
	for _, model := range catalog().Models {
		original[model.ID] = model
	}
	for _, model := range manifest.Models {
		if !reflect.DeepEqual(original[model.ID], model) {
			t.Fatal("candidate changed original model contract", model.ID)
		}
	}
	if len(manifest.Models) != 2 {
		t.Fatal("representative package must not advertise unmigrated native models")
	}
	profile, err := contract.DecodeConnectionPolicy(read("connection.profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	policies := map[string]contract.JSONRequestPolicy{}
	for name, op := range profile.Operations {
		if op.JSONPolicy == nil {
			continue
		}
		p, err := contract.DecodeJSONRequestPolicy(read("policies/" + name + ".json"))
		if err != nil {
			t.Fatal(name, err)
		}
		digest, err := p.Digest()
		if err != nil || digest != op.JSONPolicy.Digest {
			t.Fatal("profile does not bind reviewed policy", name, err)
		}
		policies[name] = p
	}
	for _, tc := range []struct {
		name, model   string
		inputs, count int
		sequential    bool
		size          string
		allowed       bool
	}{
		{"text", "wan2.7-image", 0, 1, false, "2K", true},
		{"edit-nine", "wan2.7-image", 9, 4, false, "2K", true},
		{"too-many-references", "wan2.7-image", 10, 1, false, "2K", false},
		{"batch-too-large", "wan2.7-image", 1, 5, false, "2K", false},
		{"sequential-twelve", "wan2.7-image", 9, 12, true, "2K", true},
		{"pro-4K-text", "wan2.7-image-pro", 0, 1, false, "4K", true},
		{"4K-edit", "wan2.7-image-pro", 1, 1, false, "4K", false},
		{"4K-set", "wan2.7-image-pro", 0, 1, true, "4K", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := fixture(t, New(), tc.model, map[string]any{"n": tc.count, "enable_sequential": tc.sequential, "size": tc.size}, tc.inputs)
			nativeAllowed := New().Validate(t.Context(), request) == nil
			var generation aigc.Generation
			if json.Unmarshal(request.Payload, &generation) != nil {
				t.Fatal("fixture generation")
			}
			for _, model := range manifest.Models {
				if model.ID == generation.Model {
					generation.Parameters, err = aigc.NormalizeParameters(model, generation.Parameters)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			payload, _ := json.Marshal(generation)
			candidateAllowed := aigc.ValidateGeneration(manifest, aigc.Request{Payload: payload, Inputs: request.Inputs}) == nil
			values := contract.JSONRequestValues{Generation: generation, Count: tc.count, Receipts: map[string]json.RawMessage{"original-policy": json.RawMessage(`{"data":{"upload_dir":"tenant/original"}}`)}}
			for _, input := range request.Inputs {
				sum := sha256.Sum256(input.Data)
				values.Inputs = append(values.Inputs, contract.RequestInput{Kind: "image", Mime: "image/png", SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(input.Data)), Data: input.Data})
			}
			matched := 0
			for name, p := range policies {
				if profile.Operations[name].Effect != "submit" {
					continue
				}
				if _, err := p.Build(values); err == nil {
					matched++
				}
			}
			candidateAllowed = candidateAllowed && matched == 1
			if nativeAllowed != tc.allowed || candidateAllowed != nativeAllowed {
				t.Fatal("native/public admission mismatch", nativeAllowed, candidateAllowed, "matching mappings", matched)
			}
		})
	}
}
