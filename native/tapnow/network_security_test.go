package tapnow

import (
	"context"
	"testing"
)

func TestPublicMediaRejectsPrivateAddress(t *testing.T) {
	if err := (&Client{}).PublicURL(context.Background(), "https://100.64.0.1/image.png"); err == nil {
		t.Fatal("private media URL accepted")
	}
}
