package dashscope

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestQwenAudioCloneUploadAndCreationAreSingleSubmissions(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rejected-upload"}[reject], func(t *testing.T) {
			p := New()
			calls := 0
			sample := sampleWAV()
			p.Client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				body := ""
				status := 200
				switch calls {
				case 1:
					if r.Method != "GET" || r.URL.Path != "/api/v1/uploads" || r.URL.Query().Get("model") != "voice-enrollment" || r.Header.Get("Authorization") == "" {
						t.Fatal("policy request")
					}
					body = `{"data":{"upload_host":"https://fixture.oss-cn-beijing.aliyuncs.com","upload_dir":"private/fixture","oss_access_key_id":"temporary-oss-key","policy":"policy","signature":"signature","x_oss_object_acl":"private","x_oss_forbid_overwrite":"true"}}`
				case 2:
					if r.Header.Get("Authorization") != "" || r.URL.Host != "fixture.oss-cn-beijing.aliyuncs.com" {
						t.Fatal("API credential forwarded to OSS")
					}
					if err := r.ParseMultipartForm(2 << 20); err != nil {
						t.Fatal(err)
					}
					f, _, err := r.FormFile("file")
					if err != nil {
						t.Fatal(err)
					}
					b, _ := io.ReadAll(f)
					f.Close()
					r.MultipartForm.RemoveAll()
					if !bytes.Equal(b, sample) || !strings.HasSuffix(r.FormValue("key"), ".wav") {
						t.Fatal("sample changed")
					}
					if reject {
						status = 403
					}
				case 3:
					var b struct {
						Model string
						Input map[string]any
					}
					if json.NewDecoder(r.Body).Decode(&b) != nil {
						t.Fatal("body")
					}
					if b.Model != "voice-enrollment" || b.Input["action"] != "create_voice" || !strings.HasPrefix(b.Input["url"].(string), "oss://private/fixture/") || r.Header.Get("X-DashScope-OssResourceResolve") != "enable" {
						t.Fatal("wire model or OSS resolution")
					}
					body = `{"output":{"voice_id":"qwen-audio-3.1-tts-flash-vc-fixture","target_model":"qwen-audio-3.1-tts-flash"}}`
				default:
					t.Fatal("unexpected retry")
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			r := fixture(t, p, "voice-enrollment-clone", nil, 0)
			r.Inputs = []aigc.Input{{Kind: "audio", Data: sample}}
			out, err := p.Execute(context.Background(), r, nil)
			if reject {
				if err == nil || calls != 2 {
					t.Fatal("must stop after failed upload", calls, err)
				}
				return
			}
			if err != nil || calls != 3 || len(out.Media) != 1 || out.Media[0].Voice.Kind != "clone" || !bytes.Equal(out.Media[0].Data, sample) {
				t.Fatal("clone result", calls, err)
			}
		})
	}
}
