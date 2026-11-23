package dashscope

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

func audioSample(b []byte) (string, string, error) {
	mime := http.DetectContentType(b)
	ext := ""
	switch mime {
	case "audio/wave", "audio/x-wav", "audio/wav":
		mime, ext = "audio/wav", ".wav"
	case "audio/mpeg":
		ext = ".mp3"
	}
	if len(b) > 10<<20 || len(b) == 0 || ext == "" {
		return "", "", errors.New("音色复刻需要10MB以内的WAV或MP3音频")
	}
	return mime, ext, nil
}
func (p *Plugin) uploadAudio(ctx context.Context, base, key, model string, b []byte) (string, error) {
	_, ext, err := audioSample(b)
	if err != nil {
		return "", err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/uploads?action=getPolicy&model="+url.QueryEscape(model), nil)
	req.Header.Set("Authorization", "Bearer "+key)
	res, err := p.Client.Do(req)
	if err != nil {
		return "", &Failure{}
	}
	defer res.Body.Close()
	var out struct {
		Data struct {
			Host      string `json:"upload_host"`
			Dir       string `json:"upload_dir"`
			Key       string `json:"oss_access_key_id"`
			Policy    string `json:"policy"`
			Signature string `json:"signature"`
			ACL       string `json:"x_oss_object_acl"`
			Forbid    string `json:"x_oss_forbid_overwrite"`
		} `json:"data"`
	}
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out) != nil {
		return "", &Failure{}
	}
	u, err := url.Parse(out.Data.Host)
	if err != nil || u.Scheme != "https" || u.User != nil || !strings.HasSuffix(u.Hostname(), ".aliyuncs.com") || !strings.Contains(u.Hostname(), ".oss-") || out.Data.Dir == "" || strings.Contains(out.Data.Dir, "..") || out.Data.Key == "" || out.Data.Policy == "" || out.Data.Signature == "" {
		return "", &Failure{}
	}
	sum := sha256.Sum256(b)
	name := hex.EncodeToString(sum[:]) + ext
	object := strings.TrimRight(out.Data.Dir, "/") + "/" + name
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	for k, v := range map[string]string{"OSSAccessKeyId": out.Data.Key, "policy": out.Data.Policy, "Signature": out.Data.Signature, "key": object, "x-oss-object-acl": out.Data.ACL, "x-oss-forbid-overwrite": out.Data.Forbid, "success_action_status": "200"} {
		if err = form.WriteField(k, v); err != nil {
			return "", err
		}
	}
	file, err := form.CreateFormFile("file", name)
	if err != nil {
		return "", err
	}
	if _, err = file.Write(b); err != nil {
		return "", err
	}
	if err = form.Close(); err != nil {
		return "", err
	}
	upload, _ := http.NewRequestWithContext(ctx, http.MethodPost, out.Data.Host, bytes.NewReader(buf.Bytes()))
	upload.Header.Set("Content-Type", form.FormDataContentType())
	response, err := p.Client.Do(upload)
	if err != nil {
		return "", &Failure{}
	}
	defer response.Body.Close()
	if response.StatusCode != 200 && response.StatusCode != 204 {
		return "", &Failure{}
	}
	return "oss://" + object, nil
}
