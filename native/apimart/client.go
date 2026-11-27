// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"gitea.nkit.top/xurx/Ordiweft/pkg/aigc"
)

const maxReply = 32 << 20

var taskIDPattern = regexp.MustCompile(`^task_[A-Za-z0-9_-]{1,150}$`)

type envelope struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

type remoteTask struct {
	ID          string       `json:"id"`
	TaskID      string       `json:"task_id"`
	Status      string       `json:"status"`
	Progress    float64      `json:"progress"`
	Created     int64        `json:"created"`
	Cost        *json.Number `json:"cost"`
	CreditsCost *json.Number `json:"credits_cost"`
	Usage       struct {
		CompletionTokens *int64 `json:"completion_tokens"`
	} `json:"usage"`
	Result json.RawMessage `json:"result"`
}

func (p *plugin) request(ctx context.Context, method, path, key, contentType string, body []byte, responseVersion ...string) ([]byte, error) {
	resp, err := p.doRequest(ctx, method, path, key, contentType, body, responseVersion...)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("APIMart %s HTTP %d", method, resp.StatusCode)
	}
	reply, err := io.ReadAll(io.LimitReader(resp.Body, maxReply+1))
	if err != nil || len(reply) > maxReply {
		return nil, errors.New("APIMart response unavailable or too large")
	}
	return reply, nil
}

func (p *plugin) doRequest(ctx context.Context, method, path, key, contentType string, body []byte, responseVersion ...string) (*http.Response, error) {
	var source io.Reader
	if body != nil {
		source = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, source)
	if err != nil {
		return nil, err
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if len(responseVersion) != 0 && responseVersion[0] != "" {
		req.Header.Set("X-APIMart-Response-Version", responseVersion[0])
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("APIMart %s request failed: %w", method, err)
	}
	return resp, nil
}

func referenceValue(ref reference, url string, ordinal int) (any, string) {
	switch ref.Field {
	case "ref_images":
		tag := fmt.Sprintf("@Image%d", ordinal)
		return map[string]any{"tag": tag, "type": ref.ItemRole, "image_urls": []string{url}}, tag
	case "ref_videos":
		tag := fmt.Sprintf("@Video%d", ordinal)
		return map[string]string{"tag": tag, "type": ref.ItemRole, "video_url": url}, tag
	case "mid_frame_images":
		tag := fmt.Sprintf("@Image%d", ordinal)
		return map[string]any{"tag": tag, "image_url": url, "time_stamp": -1}, tag
	case "video_list":
		return map[string]string{"video_url": url, "refer_type": ref.ItemRole, "keep_original_sound": "no"}, ""
	}
	if ref.ItemRole != "" {
		return map[string]string{"url": url, "role": ref.ItemRole}, ""
	}
	return url, ""
}

func (p *plugin) Execute(ctx context.Context, request aigc.Request, checkpoint func(aigc.Result)) (aigc.Result, error) {
	var empty aigc.Result
	key := request.Secrets[secret]
	if key == "" || strings.ContainsAny(key, " \r\n\t") {
		return empty, errors.New("invalid APIMart API key")
	}
	spec, body, files, err := p.prepare(request)
	if err != nil {
		return empty, err
	}
	if spec.Model.TaskContinuation != nil && (request.SourceTask == nil || len(files) > 0) {
		return empty, errors.New("APIMart continuation requires the bound original task")
	}
	if spec.Model.Capability == "speech.transcribe" {
		return p.transcribe(ctx, spec, key, body, files[0].input)
	}
	urls, uploads, err := p.referenceURLs(ctx, key, request.Identity, files)
	if err != nil {
		switch err {
		case errUploadQuota, errUploadThrottle, errUploadUnconfirmed, errUploadInvalid:
			empty.Metadata, _ = json.Marshal(map[string]string{"uploadErrorCode": err.Error()})
		}
		return empty, err
	}
	grouped := map[string][]any{}
	ordinals := map[string]int{}
	originalSound := body["keep_original_sound"]
	var actorVoice string
	boundVoices := map[string]string{}
	wanImages := map[string]map[string]string{}
	frameTimes, _ := body["mid_frame_times"].(map[string]any)
	delete(body, "mid_frame_times")
	for i, file := range files {
		if file.ref.BindTo != "" {
			boundVoices[file.ref.BindTo] = urls[i]
			continue
		}
		if file.ref.Field == "ref_images.audio_url" {
			actorVoice = urls[i]
			continue
		}
		ordinals[file.ref.Kind]++
		value, tag := referenceValue(file.ref, urls[i], ordinals[file.ref.Kind])
		if tag != "" {
			prompt, _ := body["prompt"].(string)
			if file.ref.Field == "ref_images" && file.ref.Role == "subject_image" {
				prompt = strings.ReplaceAll(prompt, "@Audio1", tag)
				body["prompt"] = prompt
			}
			if !strings.Contains(prompt, tag) {
				body["prompt"] = strings.TrimSpace(prompt + " " + tag)
			}
		}
		if spec.Model.ID == "wan2.7-r2v" && file.ref.Field == "image_with_roles" {
			wanImages[file.input.Role] = value.(map[string]string)
		}
		if file.ref.Field == "mid_frame_images" {
			if time, ok := frameTimes[file.input.Role]; ok {
				value.(map[string]any)["time_stamp"] = time
			}
		}
		if file.ref.Field == "video_list" && originalSound != nil {
			value.(map[string]string)["keep_original_sound"] = originalSound.(string)
			delete(body, "keep_original_sound")
		}
		if file.ref.Array {
			grouped[file.ref.Field] = append(grouped[file.ref.Field], value)
		} else {
			if key, property, nested := strings.Cut(file.ref.Field, "."); nested {
				body[key] = map[string]any{property: value}
			} else {
				body[file.ref.Field] = value
			}
		}
	}
	for role, address := range boundVoices {
		wanImages[role]["reference_voice"] = address
	}
	if actorVoice != "" {
		images := grouped["ref_images"]
		if len(images) != 1 {
			return empty, errors.New("a SkyReels voice requires one reference subject")
		}
		images[0].(map[string]any)["audio_url"] = actorVoice
	}
	for field, values := range grouped {
		body[field] = values
	}
	limit := 16
	if len(spec.Model.OutputCount) > 0 {
		var g aigc.Generation
		if err := json.Unmarshal(request.Payload, &g); err != nil {
			return empty, err
		}
		limit, err = aigc.GenerationOutputLimit(spec.Model, g.Parameters, 1)
		if err != nil {
			return empty, err
		}
	}
	var origins []aigc.TaskOrigin
	payload, err := json.Marshal(body)
	if err != nil {
		return empty, err
	}
	var responseVersion string
	if spec.Upstream == "grok-imagine-image-2.0" {
		responseVersion = "2026-07-27"
	}
	reply, err := p.request(ctx, http.MethodPost, spec.Endpoint, key, "application/json", payload, responseVersion)
	if err != nil {
		return empty, err
	}
	if spec.Endpoint == "/v1/chat/completions" {
		return decodeChatAudio(reply)
	}
	if spec.Model.Capability == "speech.generate" {
		if len(reply) == 0 || json.Valid(reply) {
			return empty, errors.New("APIMart returned no synthesized audio")
		}
		return aigc.Result{Media: []aigc.Media{{Kind: "audio", Data: reply}}}, nil
	}
	tasks, err := submission(reply)
	if err != nil {
		return empty, err
	}
	path := "/v1/tasks/"
	if spec.MusicTask {
		path = "/v1/music/tasks/"
	}
	if checkpoint != nil {
		checkpoint(aigc.Result{Metadata: taskMetadata(spec, path, tasks, uploads)})
	}
	taskList := make([]remoteTask, 0, len(tasks))
	var all []aigc.Media
	sourceEligible := p.manifest.CanProduceTaskSource(spec.Model.ID, body)
	for _, id := range tasks {
		task, err := p.wait(ctx, key, path, id)
		if err != nil {
			return empty, err
		}
		media, err := decodeTaskMedia(spec, body, task.Result)
		if err != nil {
			return empty, err
		}
		taskList = append(taskList, task)
		// APIMart's follow-up endpoints select a task, with no field for
		// choosing between several outputs belonging to the same task.
		if sourceEligible && singleVideoOutput(media) && len(all) < 16 &&
			task.Created > 0 && task.Created <= time.Now().Add(5*time.Minute).Unix() {
			origins = append(origins, aigc.TaskOrigin{
				SourceTask: aigc.SourceTask{Ticket: task.ID, OutputIndex: len(all)}, CreatedAt: task.Created * 1000,
			})
		}
		all = append(all, media...)
	}
	if len(all) == 0 || len(all) > limit {
		return empty, errors.New("APIMart returned an unsupported number of outputs")
	}
	meta := map[string]any{
		"taskIds": tasks, "taskQueryPath": path, "endpoint": spec.Endpoint,
		"upstreamModel": spec.Upstream,
	}
	if uploads != nil {
		meta["referenceUploads"] = uploads
	}
	receipts := make([]map[string]any, 0, len(taskList))
	var completionTokens int64
	usageComplete := true
	for _, task := range taskList {
		receipts = append(receipts, map[string]any{
			"taskId": task.ID, "usd": task.Cost, "credits": task.CreditsCost,
		})
		if tokens := task.Usage.CompletionTokens; tokens != nil && *tokens >= 0 && *tokens <= 1e9 {
			completionTokens += *tokens
		} else {
			usageComplete = false
		}
	}
	meta["receipts"] = receipts
	if usageComplete {
		meta["usage"] = map[string]int64{"completion_tokens": completionTokens}
	}
	resultMetadata, _ := json.Marshal(meta)
	return aigc.Result{Media: all, Metadata: resultMetadata, TaskOrigins: origins}, nil
}

func decodeChatAudio(reply []byte) (aigc.Result, error) {
	var response struct {
		Choices []struct {
			Message struct {
				Audio struct {
					Data []byte `json:"data"`
				} `json:"audio"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(reply, &response) != nil || len(response.Choices) != 1 || len(response.Choices[0].Message.Audio.Data) == 0 {
		return aigc.Result{}, errors.New("APIMart audio chat returned no audio")
	}
	return aigc.Result{Media: []aigc.Media{{Kind: "audio", Data: response.Choices[0].Message.Audio.Data}}}, nil
}

func taskMetadata(spec modelSpec, path string, tasks []string, uploads *uploadSummary) json.RawMessage {
	fields := map[string]any{
		"taskIds": tasks, "taskQueryPath": path, "endpoint": spec.Endpoint,
		"upstreamModel": spec.Upstream,
	}
	if uploads != nil {
		fields["referenceUploads"] = uploads
	}
	meta, _ := json.Marshal(fields)
	return meta
}

func submission(reply []byte) ([]string, error) {
	var response envelope
	var tasks []remoteTask
	if json.Unmarshal(reply, &response) != nil {
		return nil, errors.New("APIMart returned an invalid submission response")
	}
	if response.Code == 202 {
		var task struct {
			ID     string `json:"id"`
			Object string `json:"object"`
		}
		if json.Unmarshal(response.Data, &task) != nil || task.Object != "generation.task" || !taskIDPattern.MatchString(task.ID) {
			return nil, errors.New("APIMart returned an invalid versioned submission receipt")
		}
		return []string{task.ID}, nil
	}
	if response.Code != 200 || json.Unmarshal(response.Data, &tasks) != nil || len(tasks) == 0 || len(tasks) > 16 {
		return nil, errors.New("APIMart returned no valid submission receipt")
	}
	ids := make([]string, 0, len(tasks))
	seen := map[string]bool{}
	for _, task := range tasks {
		if !taskIDPattern.MatchString(task.TaskID) {
			return nil, errors.New("APIMart returned an invalid task ID")
		}
		if !seen[task.TaskID] {
			ids = append(ids, task.TaskID)
			seen[task.TaskID] = true
		}
	}
	return ids, nil
}

func (p *plugin) wait(ctx context.Context, key, path, id string) (remoteTask, error) {
	var task remoteTask
	for {
		if err := p.query(ctx, key, path, id, &task); err != nil {
			return task, err
		}
		switch task.Status {
		case "completed":
			return task, nil
		case "failed", "cancelled", "canceled":
			return task, fmt.Errorf("APIMart task %s ended with status %s", id, task.Status)
		case "submitted", "queued", "pending", "processing", "running", "unknown", "in_progress":
		default:
			return task, errors.New("APIMart returned an unrecognized task status")
		}
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return task, ctx.Err()
		case <-timer.C:
		}
	}
}

func (p *plugin) query(ctx context.Context, key, path, id string, task *remoteTask) error {
	reply, err := p.request(ctx, http.MethodGet, path+id+"?language=zh", key, "", nil)
	if err != nil {
		return err
	}
	var response envelope
	if json.Unmarshal(reply, &response) != nil || response.Code != 200 ||
		json.Unmarshal(response.Data, task) != nil || task.ID != id {
		return errors.New("APIMart query did not return the original task")
	}
	return nil
}

func (p *plugin) printStatus(ctx context.Context, args []string) error {
	if len(args) != 2 || !taskIDPattern.MatchString(args[1]) {
		return errors.New("usage: provider status <model-id> <task-id>")
	}
	spec, ok := p.models[args[0]]
	if !ok {
		return errors.New("unknown APIMart model")
	}
	key := os.Getenv("APIMART_API_KEY")
	if key == "" || strings.ContainsAny(key, " \r\n\t") {
		return errors.New("invalid APIMart API key")
	}
	path := "/v1/tasks/"
	if spec.MusicTask {
		path = "/v1/music/tasks/"
	}
	var task remoteTask
	if err := p.query(ctx, key, path, args[1], &task); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(task)
}

type remoteMedia struct {
	URL      json.RawMessage `json:"url"`
	AudioURL string          `json:"audio_url"`
	VideoURL string          `json:"video_url"`
}

func decodeMedia(capability string, raw json.RawMessage) ([]aigc.Media, error) {
	var result struct {
		Images   []remoteMedia `json:"images"`
		Videos   []remoteMedia `json:"videos"`
		Music    []remoteMedia `json:"music"`
		Audios   []remoteMedia `json:"audios"`
		URL      string        `json:"url"`
		ImageURL string        `json:"image_url"`
		AudioURL string        `json:"audio_url"`
		VideoURL string        `json:"video_url"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return nil, errors.New("invalid APIMart task result")
	}
	var rows []remoteMedia
	var kind, fallback string
	switch capability {
	case "image.generate":
		kind, rows, fallback = "image", result.Images, result.ImageURL
	case "video.generate":
		kind, rows, fallback = "video", result.Videos, result.VideoURL
		if len(rows) == 0 {
			rows = result.Music
		}
	case "music.generate":
		kind, rows, fallback = "audio", result.Music, result.AudioURL
		rows = append(rows, result.Audios...)
	default:
		return nil, errors.New("unsupported APIMart output kind")
	}
	if fallback == "" {
		fallback = result.URL
	}
	var media []aigc.Media
	seen := map[string]bool{}
	appendURL := func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
			return errors.New("APIMart returned an invalid HTTPS media URL")
		}
		if !seen[raw] {
			media = append(media, aigc.Media{Kind: kind, URL: raw})
			seen[raw] = true
		}
		return nil
	}
	for _, row := range rows {
		var urls []string
		if len(row.URL) != 0 && json.Unmarshal(row.URL, &urls) != nil {
			var one string
			if json.Unmarshal(row.URL, &one) != nil {
				return nil, errors.New("invalid APIMart output URLs")
			}
			urls = []string{one}
		}
		if kind == "audio" && row.AudioURL != "" {
			urls = []string{row.AudioURL}
		}
		if kind == "video" && row.VideoURL != "" {
			urls = []string{row.VideoURL}
		}
		for _, u := range urls {
			if u != "" {
				if err := appendURL(u); err != nil {
					return nil, err
				}
			}
		}
	}
	if len(media) == 0 && fallback != "" {
		if err := appendURL(fallback); err != nil {
			return nil, err
		}
	}
	return media, nil
}

func httpsURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}

func referenceType(input aigc.Input) (string, string, error) {
	if len(input.Data) == 0 {
		if !httpsURL(input.URL) {
			return "", "", errors.New("reference requires bytes or a public HTTPS URL")
		}
		return "", "", nil
	}
	contentType := http.DetectContentType(input.Data)
	type format struct{ kind, mime, ext string }
	formats := map[string]format{
		"image/png": {"image", "image/png", ".png"}, "image/jpeg": {"image", "image/jpeg", ".jpg"},
		"image/webp": {"image", "image/webp", ".webp"}, "image/gif": {"image", "image/gif", ".gif"},
		"video/mp4": {"video", "video/mp4", ".mp4"}, "video/webm": {"video", "video/webm", ".webm"},
		"audio/wave": {"audio", "audio/wav", ".wav"}, "audio/mpeg": {"audio", "audio/mpeg", ".mp3"},
	}
	f := formats[contentType]
	if input.Kind == "audio" && len(input.Data) >= 4 && input.Data[0] == 0xff &&
		input.Data[1]&0xe0 == 0xe0 && input.Data[1]&0x06 == 0x02 && input.Data[2]&0xf0 != 0xf0 &&
		input.Data[2]&0x0c != 0x0c {
		f = format{"audio", "audio/mpeg", ".mp3"}
	}
	if len(input.Data) >= 12 && string(input.Data[4:8]) == "ftyp" {
		switch brand := string(input.Data[8:12]); {
		case input.Kind == "audio":
			if brand == "M4A " || brand == "isom" || brand == "mp42" {
				f = format{"audio", "audio/mp4", ".m4a"}
			}
		case brand == "qt  ":
			f = format{"video", "video/quicktime", ".mov"}
		}
	}
	if f.kind == "" || f.kind != input.Kind {
		return "", "", errors.New("reference type is unsupported by APIMart")
	}
	return f.mime, f.ext, nil
}

func multipartFile(data []byte, extension string, fields map[string]string) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			return nil, "", err
		}
	}
	part, err := writer.CreateFormFile("file", "reference"+extension)
	if err == nil {
		_, err = part.Write(data)
	}
	if err == nil {
		err = writer.Close()
	}
	return body.Bytes(), writer.FormDataContentType(), err
}
