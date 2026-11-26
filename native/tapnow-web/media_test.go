package tapnowweb

import (
	"context"
	"testing"
)

func TestMaterialUsesBytesInsteadOfClaims(t *testing.T) {
	m, e := inspectMaterial(Material{Bytes: fixtureImage(), Mime: "image/jpeg"}, "image")
	if e != nil || m.Mime != "image/png" {
		t.Fatal(m, e)
	}
	m, e = inspectMaterial(Material{Bytes: fixtureVideo(5), Mime: "video/mp4", Duration: 30}, "video")
	if e != nil || m.Duration != 5 {
		t.Fatal(m.Duration, e)
	}
	c, count, _ := fixture(t, "2", false)
	if _, e = c.Upload(context.Background(), Material{Bytes: []byte("<html>not png</html>"), Mime: "image/png"}); e == nil || count.Load() != 0 {
		t.Fatal("claimed MIME reached paid generation")
	}
}
