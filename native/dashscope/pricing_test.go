package dashscope

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestPricingManifestIdentityIsStableAcrossInstances(t *testing.T) {
	first := (&Plugin{region: "cn-beijing"}).Manifest()
	if e := first.Validate(); e != nil {
		t.Fatal(e)
	}
	want, e := json.Marshal(first)
	if e != nil {
		t.Fatal(e)
	}
	for range 100 {
		got, e := json.Marshal((&Plugin{region: "cn-beijing"}).Manifest())
		if e != nil || !bytes.Equal(got, want) {
			t.Fatal("equivalent channel instances produced different frozen plugin identities", e)
		}
	}
}
