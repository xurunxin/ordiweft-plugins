package comfyui

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
	"golang.org/x/net/websocket"
)

func TestLiveProgressIsScopedToTicketAndKeepsSingleSubmission(t *testing.T) {
	var submits, queueReads atomic.Int32
	var stepsSeen atomic.Bool
	var socket *websocket.Conn
	ready := make(chan struct{})
	var picture bytes.Buffer
	_ = png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 7, 11)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws":
			websocket.Handler(func(conn *websocket.Conn) {
				socket = conn
				close(ready)
				var ignored []byte
				_ = websocket.Message.Receive(conn, &ignored)
			}).ServeHTTP(w, r)
		case "/upload/image":
			w.Write([]byte(`{"name":"uploaded.png","type":"input"}`))
		case "/prompt":
			<-ready
			submits.Add(1)
			w.Write([]byte(`{"prompt_id":"own","node_errors":{}}`))
		case "/queue":
			n := queueReads.Add(1)
			if n == 1 {
				w.Write([]byte(`{"queue_running":[[1,"someone-else",{"secret":"do-not-leak"}]],"queue_pending":[[3,"own"],[2,"other-private"]]}`))
				return
			}
			w.Write([]byte(`{"queue_running":[[3,"own"]],"queue_pending":[]}`))
			if n == 2 {
				_ = websocket.Message.Send(socket, `{"type":"executing","data":{"prompt_id":"own","node":"save"}}`)
				_ = websocket.Message.Send(socket, `{"type":"progress","data":{"prompt_id":"own","value":3,"max":8}}`)
				_ = websocket.Message.Send(socket, `{"type":"progress","data":{"prompt_id":"other-private","value":999,"max":1000}}`)
			}
		case "/history/own":
			if !stepsSeen.Load() {
				w.Write([]byte(`{}`))
				return
			}
			w.Write([]byte(`{"own":{"status":{"completed":true,"status_str":"success"},"outputs":{"save":{"images":[{"filename":"result.png","type":"output"}]}}}}`))
		case "/view":
			w.Write(picture.Bytes())
		default:
			t.Error("unexpected route", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	d, err := Compile("fixture", server.URL, json.RawMessage(fixtureGraph), fixtureInfo())
	if err != nil {
		t.Fatal(err)
	}
	p := New(d)
	p.PollInterval = 5 * time.Millisecond
	payload, _ := json.Marshal(aigc.Generation{Provider: d.Manifest.ID, Model: "workflow", Capability: "image.generate"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stages := map[string]bool{}
	_, err = p.Execute(ctx, aigc.Request{JobID: "progress-test", Payload: payload, Inputs: []aigc.Input{{Kind: "image", Role: "ref-1", Data: picture.Bytes()}}}, func(result aigc.Result) {
		if bytes.Contains(result.Metadata, []byte("private")) || bytes.Contains(result.Metadata, []byte("do-not-leak")) {
			t.Error("other task information leaked")
		}
		var metadata struct {
			PromptID string   `json:"promptId"`
			Progress Progress `json:"progress"`
		}
		_ = json.Unmarshal(result.Metadata, &metadata)
		if metadata.PromptID != "own" {
			t.Error("wrong ticket")
		}
		stages[metadata.Progress.Stage] = true
		if metadata.Progress.Stage == "queued" && (metadata.Progress.QueuePosition != 2 || metadata.Progress.Ahead != 2) {
			t.Error("queue ordering wrong", metadata.Progress)
		}
		if metadata.Progress.Value == 999 {
			t.Error("foreign progress accepted")
		}
		if metadata.Progress.Value == 3 && metadata.Progress.Max == 8 {
			stepsSeen.Store(true)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if submits.Load() != 1 || !stepsSeen.Load() || !stages["queued"] || !stages["running"] || !stages["receiving"] {
		t.Fatal("progress stages missing or resubmission", stages, submits.Load())
	}
}
