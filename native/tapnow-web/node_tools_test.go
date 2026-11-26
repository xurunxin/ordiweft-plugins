package tapnowweb

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"gitea.nkit.top/xurx/Ordiweft/internal/tapnow"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func toolWAV() []byte {
	b := make([]byte, 44+32000)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 16000)
	binary.LittleEndian.PutUint32(b[28:], 32000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], 32000)
	return b
}
func TestLensUsesNativeRouteAndSavedConfiguration(t *testing.T) {
	cat := Baseline()
	_, mode, _ := cat.Find("gpt-image-2.5-flare", "image", "text_to_image")
	cfg := &CameraControl{true, "sony_venice", "zeiss_ultra_prime", "24mm", "f/4"}
	r := Request{Model: "gpt-image-2.5-flare", Mode: mode.ID, Kind: "image", Prompt: "fixture", Parameters: defaults(mode), CameraControl: cfg}
	body, _, e := cat.Prepare(r)
	if e != nil || body["cameraControl"] != cfg || generationPath(r) != "/api/bff/generation/image/camera_control" {
		t.Fatal(body, e)
	}
	if EstimateBody("image", body)["imageParams"].(map[string]any)["cameraControl"] != nil {
		t.Fatal("camera leaked into ordinary billing fields")
	}
	for _, invalid := range []CameraControl{{true, "not-a-camera", "zeiss_ultra_prime", "24mm", "f/4"}, {true, "sony_venice", "not-a-lens", "24mm", "f/4"}, {true, "sony_venice", "zeiss_ultra_prime", "999mm", "f/4"}, {true, "sony_venice", "zeiss_ultra_prime", "24mm", "f/99"}} {
		r.CameraControl = &invalid
		if _, _, e := cat.Prepare(r); e == nil {
			t.Fatal("invalid Lens accepted")
		}
	}
	r.CameraControl = cfg
	r.Model = "doubao-seedream-4.5"
	if validateCamera(r) == nil {
		t.Fatal("unsupported camera model")
	}
	cfg.Enabled = false
	if generationPath(r) != "/api/conversation/v1/generations/image" {
		t.Fatal("disabled Lens route")
	}
}
func TestGoogleProLensRequiresSearchAsOnWebsite(t *testing.T) {
	cat := Baseline()
	_, mode, e := cat.Find("tamar-google-gemini-pro", "image", "text_to_image")
	if e != nil {
		t.Fatal(e)
	}
	r := Request{Model: "tamar-google-gemini-pro", Mode: mode.ID, Kind: "image", Prompt: "fixture", Parameters: defaults(mode), CameraControl: &CameraControl{true, "sony_venice", "zeiss_ultra_prime", "24mm", "f/4"}}
	r.Parameters["enableGoogleSearch"] = false
	if _, _, e := cat.Prepare(r); e == nil {
		t.Fatal("Lens accepted without required Google search")
	}
	r.Parameters["enableGoogleSearch"] = true
	if _, _, e := cat.Prepare(r); e != nil {
		t.Fatal(e)
	}
}
func TestMultimodalModelNativeProjectionAndNoUnresolvedToken(t *testing.T) {
	cat := Baseline()
	_, mode, _ := cat.Find("seedance-2.5", "video", "reference_to_video")
	r := Request{Model: "seedance-2.5", Mode: mode.ID, Kind: "video", Prompt: "{{Image 1}} {{Video 1}} {{Audio 1}}", Parameters: defaults(mode), VideoConsent: true, UploadConsent: true, References: []tapnow.Reference{{Kind: "audio", Role: "audio", URL: "/files/wav", Order: 2}, {Kind: "video", Role: "video", URL: "/files/mp4", Order: 1}, {Kind: "image", Role: "image", URL: "/files/png", Order: 0}}}
	body, _, e := cat.Prepare(r)
	if e != nil || body["prompt"] != "Image1 Video1 Audio1" {
		t.Fatal(body, e)
	}
	b, _ := json.Marshal(body["audios"])
	if string(b) != `[{"is_verified":false,"role":"reference_audio","url":"/files/wav"}]` {
		t.Fatal(string(b))
	}
	for _, s := range []string{"@{not-converted}", "{{Video 2}}", "{{Audio 0}}", "{{node/foo}}"} {
		r.Prompt = s
		if _, _, e := cat.Prepare(r); e == nil {
			t.Fatal("invalid token accepted", s)
		}
	}
	r.Prompt = "{{Audio 1}}"
	r.References = r.References[:1]
	if _, _, e := cat.Prepare(r); e == nil {
		t.Fatal("audio alone accepted")
	}
}
func TestSubjectReadSchemaAndASRNativeBodyNoWriteReplay(t *testing.T) {
	calls := 0
	asr := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Path {
		case "/api/library/v1/elements:search":
			if r.URL.Query().Get("keyword") != "a & b" || r.URL.Query().Get("page_size") != "20" || r.URL.Query().Get("space_type") != "SPACE_TYPE_PRIVATE" {
				t.Error("incorrect query")
			}
			w.Write([]byte(`{"code":0,"data":{"items":[{"element_id":"element-fixture","name":"Fixture","asset_count":2,"revision":"7"}],"pagination":{"page":1,"total":1}}}`))
		case "/api/library/v1/elements/element-fixture":
			w.Write([]byte(`{"code":0,"data":{"element_id":"element-fixture","assets":[{"asset_id":"two","asset_type":"ASSET_TYPE_VIDEO","sort_order":2},{"asset_id":"one","asset_type":"ASSET_TYPE_IMAGE","sort_order":1}]}}`))
		case "/api/conversation/v1/file/upload-url":
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			if b["mime_type"] != "audio/wav" || !strings.HasSuffix(b["file_name"].(string), ".wav") {
				t.Error("false upload MIME")
			}
			w.Write([]byte(`{"code":0,"data":{"file_id":"voice-fixture"}}`))
		case "/api/chat/v1/beta/asr/transcribe":
			asr++
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			if b["fileId"] != "voice-fixture" || b["mimeType"] != "audio/wav" || b["mode"] != "lite" {
				t.Error("incorrect native ASR body")
			}
			w.Write([]byte(`{"code":0,"data":{"text":" fixture transcript "}}`))
		default:
			t.Error("unexpected path", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	c := New("", "fixture-token", "", t.TempDir(), "")
	c.origin = srv.URL
	list, e := c.Subjects(context.Background(), "private", "a & b", 1)
	if e != nil || len(list.Items) != 1 || list.Items[0].Revision != "7" {
		t.Fatal(list, e)
	}
	detail, e := c.Subject(context.Background(), "element-fixture")
	if e != nil || detail.Assets[0].ID != "one" {
		t.Fatal(detail, e)
	}
	prior := calls
	if _, e := c.Transcribe(context.Background(), Material{Bytes: []byte("fake.wav"), Mime: "audio/wav"}); e == nil || calls != prior {
		t.Fatal("invalid audio uploaded")
	}
	text, e := c.Transcribe(context.Background(), Material{Bytes: toolWAV()})
	if e != nil || text != "fixture transcript" || asr != 1 {
		t.Fatal(text, e, asr)
	}
}
