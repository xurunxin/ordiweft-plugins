package dashscope

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestVoiceAliasUsesSameBucketAndWireRateGate(t *testing.T) {
	p := New()
	m := p.Manifest()
	bucket := ""
	for _, model := range m.Models {
		if model.ID == "voice-enrollment" {
			bucket = model.QueueLimit.Bucket
		}
		if model.ID == "voice-enrollment-clone" && model.QueueLimit.Bucket != bucket {
			t.Fatal("clone alias split bucket")
		}
	}
	start := time.Now()
	if e := p.waitSubmission(context.Background(), "voice-enrollment"); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if e := p.waitSubmission(ctx, "voice-enrollment"); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("wire gate ignored canceled wait", e)
	}
	// The clone alias must wait on the already consumed shared bucket too.
	if e := p.waitSubmission(context.Background(), "voice-enrollment-clone"); e != nil || time.Since(start) < 100*time.Millisecond {
		t.Fatal("alias bypassed shared wire gate", e)
	}
}
