package ark

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/setting"
)

// VideoTaskAdaptor translates the gateway task API to Ark's asynchronous API.
type VideoTaskAdaptor struct{}

// IsVideoModel recognizes the domestic and BytePlus Seedance model families.
func IsVideoModel(name string) bool {
	return strings.HasPrefix(name, "doubao-seedance-") || strings.HasPrefix(name, "dreamina-seedance-") || strings.HasPrefix(name, "seedance-")
}

func (VideoTaskAdaptor) Name() string     { return "ark-video" }
func (VideoTaskAdaptor) Platform() string { return "ark-video" }
func (VideoTaskAdaptor) SubmitURL(base string) string {
	return APIBaseURL(base) + "/contents/generations/tasks"
}
func (VideoTaskAdaptor) FetchURL(base, id string) string {
	return APIBaseURL(base) + "/contents/generations/tasks/" + url.PathEscape(id)
}
func (VideoTaskAdaptor) PrepareRequest(req *http.Request, key string, body []byte) error {
	return (Adaptor{}).PrepareRequest(req, key, body)
}

var videoPromptFlags = regexp.MustCompile(`(?i)(^|\s)--(dur|duration|frames|rs|resolution|rt|ratio|n)(\s|=|$)`)
var videoTaskID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

func (VideoTaskAdaptor) ValidateRequestAndSetAction(body []byte, req *dto.TaskRequest) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || fields == nil {
		return errors.New("invalid Ark video request")
	}
	var extra map[string]json.RawMessage
	if raw := fields["extra"]; len(raw) > 0 && json.Unmarshal(raw, &extra) != nil {
		return errors.New("extra must be an object")
	}
	if extra == nil {
		extra = map[string]json.RawMessage{}
	}
	for key, value := range fields {
		switch key {
		case "model", "prompt", "seconds", "size", "n", "max_output_tokens", "extra":
			continue
		case "content", "duration", "resolution", "ratio", "generate_audio", "seed", "watermark", "camera_fixed", "return_last_frame":
			if _, exists := extra[key]; exists {
				return fmt.Errorf("duplicate Ark video parameter: %s", key)
			}
			extra[key] = value
		default:
			return fmt.Errorf("unsupported Ark video parameter: %s", key)
		}
	}
	for key, raw := range extra {
		switch key {
		case "content", "duration", "resolution", "ratio":
		case "generate_audio", "watermark", "camera_fixed", "return_last_frame":
			var value *bool
			if json.Unmarshal(raw, &value) != nil || value == nil {
				return fmt.Errorf("%s must be a boolean", key)
			}
		case "seed":
			var value *int64
			if json.Unmarshal(raw, &value) != nil || value == nil || *value < -1 || *value > 4294967295 {
				return errors.New("seed must be an integer between -1 and 4294967295")
			}
		default:
			return fmt.Errorf("unsupported Ark video parameter: %s", key)
		}
	}
	if duration, exists := extra["duration"]; exists {
		if _, exists := fields["seconds"]; exists {
			return errors.New("use either seconds or duration, not both")
		}
		fields["seconds"] = duration
		delete(extra, "duration")
	}
	for _, key := range []string{"seconds", "n"} {
		if raw, exists := fields[key]; exists {
			var value *int
			if json.Unmarshal(raw, &value) != nil || value == nil {
				return fmt.Errorf("%s must be an integer", key)
			}
		}
	}
	var prompt string
	if raw := fields["prompt"]; len(raw) > 0 && json.Unmarshal(raw, &prompt) != nil {
		return errors.New("prompt must be a string")
	}
	if videoPromptFlags.MatchString(prompt) {
		return errors.New("use JSON video parameters instead of prompt flags")
	}
	if err := validateVideoContent(extra["content"]); err != nil {
		return err
	}
	if strings.TrimSpace(prompt) == "" && len(extra["content"]) == 0 {
		return errors.New("prompt or content is required")
	}
	// The generic validator requires text; reference-only input keeps its real prompt empty.
	fields["prompt"], _ = json.Marshal("Reference media")
	var size, modelName, resolution, ratio string
	if json.Unmarshal(fields["model"], &modelName) != nil {
		return errors.New("model is required")
	}
	if raw := fields["size"]; len(raw) > 0 && json.Unmarshal(raw, &size) != nil {
		return errors.New("size must be a string")
	}
	if raw := extra["resolution"]; len(raw) > 0 && (json.Unmarshal(raw, &resolution) != nil || resolution == "") {
		return errors.New("resolution must be a string")
	}
	if raw := extra["ratio"]; len(raw) > 0 && (json.Unmarshal(raw, &ratio) != nil || ratio == "") {
		return errors.New("ratio must be a string")
	}
	if size != "" && size != "auto" {
		if resolution != "" || ratio != "" {
			return errors.New("use either size or resolution/ratio")
		}
		switch size {
		case "480p", "720p", "1080p", "4k":
			resolution = size
		case "1280x720":
			resolution, ratio = "720p", "16:9"
		case "720x1280":
			resolution, ratio = "720p", "9:16"
		case "1920x1080":
			resolution, ratio = "1080p", "16:9"
		case "1080x1920":
			resolution, ratio = "1080p", "9:16"
		case "640x640":
			resolution, ratio = "480p", "1:1"
		case "960x960":
			resolution, ratio = "720p", "1:1"
		case "1440x1440":
			resolution, ratio = "1080p", "1:1"
		default:
			return errors.New("unsupported Ark video size; use resolution and ratio")
		}
	}
	fields["size"], _ = json.Marshal("")
	maxSeconds, minSeconds := 30, 4
	family := strings.TrimPrefix(strings.TrimPrefix(modelName, "doubao-"), "dreamina-")
	if strings.HasPrefix(family, "seedance-1-") {
		maxSeconds, minSeconds = 12, 2
	}
	if strings.HasPrefix(family, "seedance-2-0-") {
		maxSeconds = 15
	}
	if resolution == "" {
		resolution = "720p"
		if strings.HasPrefix(family, "seedance-1-") {
			resolution = "1080p"
		}
	}
	pixels := map[string]int64{"480p": 430000, "720p": 970000, "1080p": 2100000, "4k": 8300000}[resolution]
	if pixels == 0 {
		return errors.New("resolution must be 480p, 720p, 1080p or 4k")
	}
	if strings.HasPrefix(family, "seedance-2-") && (strings.Contains(modelName, "-fast-") || strings.Contains(modelName, "-mini-") || strings.Contains(modelName, "-flash-")) && resolution != "480p" && resolution != "720p" {
		return errors.New("this Seedance model supports 480p or 720p")
	}
	if strings.HasPrefix(family, "seedance-1-") && resolution == "4k" {
		return errors.New("Seedance 1.x does not support 4k")
	}
	if ratio != "" {
		switch ratio {
		case "16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive":
		default:
			return errors.New("invalid Ark video ratio")
		}
		extra["ratio"], _ = json.Marshal(ratio)
	}
	extra["resolution"], _ = json.Marshal(resolution)
	fields["extra"], _ = json.Marshal(extra)
	normalized, _ := json.Marshal(fields)
	parsed, err := dto.ParseVideoTaskRequest(normalized)
	if err != nil {
		return err
	}
	if parsed.N != 1 {
		return errors.New("Ark video tasks require n=1")
	}
	if parsed.Seconds < minSeconds || parsed.Seconds > maxSeconds {
		return fmt.Errorf("seconds must be between %d and %d for this Seedance model", minSeconds, maxSeconds)
	}
	parsed.Prompt, parsed.Size = prompt, size
	zero := int64(0)
	parsed.EstimatedInputTokens = &zero
	// Admission estimates cover the largest pixel area in the requested tier.
	parsed.MaxOutputTokens = max(parsed.MaxOutputTokens, (int64(parsed.Seconds)*pixels*24+1023)/1024)
	*req = *parsed
	return nil
}

func validateVideoContent(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var content []struct {
		Type  string `json:"type"`
		Text  string `json:"text"`
		Role  string `json:"role"`
		Image *struct {
			URL string `json:"url"`
		} `json:"image_url"`
		Video *struct {
			URL string `json:"url"`
		} `json:"video_url"`
		Audio *struct {
			URL string `json:"url"`
		} `json:"audio_url"`
	}
	if json.Unmarshal(raw, &content) != nil || len(content) == 0 || len(content) > 64 {
		return errors.New("content must contain 1 to 64 text or HTTP media entries")
	}
	for _, item := range content {
		var media string
		switch item.Type {
		case "text":
			if strings.TrimSpace(item.Text) == "" || videoPromptFlags.MatchString(item.Text) {
				return errors.New("content text must be nonempty and cannot contain video parameter flags")
			}
			continue
		case "image_url":
			if item.Role != "" && item.Role != "first_frame" && item.Role != "last_frame" && item.Role != "reference_image" {
				return errors.New("invalid image reference role")
			}
			if item.Image != nil {
				media = item.Image.URL
			}
		case "video_url":
			if item.Role != "reference_video" {
				return errors.New("video references require role=reference_video")
			}
			if item.Video != nil {
				media = item.Video.URL
			}
		case "audio_url":
			if item.Role != "reference_audio" {
				return errors.New("audio references require role=reference_audio")
			}
			if item.Audio != nil {
				media = item.Audio.URL
			}
		default:
			return errors.New("unsupported Ark video content type")
		}
		if !validVideoURL(media) {
			return errors.New("video references require HTTP or HTTPS URLs without credentials")
		}
	}
	return nil
}

func validVideoURL(raw string) bool {
	if len(raw) > 8192 {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil
}

func (VideoTaskAdaptor) ConvertSubmitRequest(req *dto.TaskRequest) ([]byte, error) {
	if req.N != 1 || req.Seconds <= 0 || req.Seconds > 30 {
		return nil, errors.New("invalid Ark video request")
	}
	body := map[string]any{}
	for key, value := range req.Extra {
		body[key] = value
	}
	var content []json.RawMessage
	if raw := req.Extra["content"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &content); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(req.Prompt) != "" {
		entry, _ := json.Marshal(map[string]string{"type": "text", "text": req.Prompt})
		content = append(content, entry)
	}
	body["model"], body["content"], body["duration"] = req.Model, content, req.Seconds
	return json.Marshal(body)
}

func (VideoTaskAdaptor) ParseSubmitResponse(status int, body []byte) (string, error) {
	var result struct {
		ID    string          `json:"id"`
		Error json.RawMessage `json:"error"`
	}
	if status < 200 || status >= 300 || json.Unmarshal(body, &result) != nil || !videoTaskID.MatchString(result.ID) || (len(result.Error) > 0 && string(result.Error) != "null") {
		return "", errors.New("invalid Ark video submission response")
	}
	return result.ID, nil
}

func (VideoTaskAdaptor) ParseTaskResult(status int, body []byte) (*dto.TaskResult, error) {
	var result struct {
		Status   string          `json:"status"`
		Duration int             `json:"duration"`
		Usage    json.RawMessage `json:"usage"`
		Content  struct {
			URL string `json:"video_url"`
		} `json:"content"`
	}
	if status < 200 || status >= 300 || json.Unmarshal(body, &result) != nil || result.Duration < 0 || result.Duration > 30 {
		return nil, errors.New("invalid Ark video task response")
	}
	out := &dto.TaskResult{Seconds: result.Duration}
	switch result.Status {
	case "queued":
		out.Status = dto.TaskStatusSubmitted
	case "running":
		out.Status = dto.TaskStatusInProgress
	case "succeeded":
		if !validVideoURL(result.Content.URL) {
			return nil, errors.New("successful Ark video task has no usable video URL")
		}
		out.Status, out.Progress, out.Outputs = dto.TaskStatusSucceeded, 100, []string{result.Content.URL}
	case "failed", "expired":
		out.Status, out.Progress, out.FailReason = dto.TaskStatusFailed, 100, "Ark video task "+result.Status
	case "cancelled":
		out.Status, out.Progress, out.FailReason = dto.TaskStatusCancelled, 100, "Ark video task cancelled"
	default:
		return nil, errors.New("unknown Ark video task status")
	}
	if len(result.Usage) > 0 && string(result.Usage) != "null" {
		var usage struct {
			Completion *int64 `json:"completion_tokens"`
			Total      *int64 `json:"total_tokens"`
		}
		if json.Unmarshal(result.Usage, &usage) != nil || (usage.Completion == nil && usage.Total == nil) || (usage.Completion != nil && usage.Total != nil && *usage.Completion != *usage.Total) {
			return nil, errors.New("invalid Ark video token usage")
		}
		tokens := usage.Completion
		if tokens == nil {
			tokens = usage.Total
		}
		// Ark documents video input tokens as zero; total equals completion tokens.
		out.Usage = &dto.Usage{CompletionTokens: *tokens}
		if err := dto.ValidateTaskUsage(out.Usage); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (a VideoTaskAdaptor) EstimateBilling(model, group string, req *dto.TaskRequest) channel.TaskPricing {
	price, set := setting.ModelPriceValue(model)
	expr, _ := setting.ModelPriceExpr(model)
	return a.EstimateBillingWithOptions(model, group, req, channel.TaskPricingOptions{ModelPriceUSD: price, ModelPriceSet: set, Expression: expr})
}

func (VideoTaskAdaptor) EstimateBillingWithOptions(_, _ string, req *dto.TaskRequest, options channel.TaskPricingOptions) channel.TaskPricing {
	if options.Expression != "" {
		return channel.TaskPricing{ExprVars: map[string]float64{"seconds": float64(req.Seconds), "n": 1, "task_status": 1, "prompt_tokens": 0, "completion_tokens": float64(req.MaxOutputTokens), "total_tokens": float64(req.MaxOutputTokens)}}
	}
	if options.ModelPriceSet {
		return channel.TaskPricing{PerCallUSD: options.ModelPriceUSD, Units: float64(req.Seconds)}
	}
	return channel.TaskPricing{CompletionTokens: req.MaxOutputTokens}
}

func (VideoTaskAdaptor) AdjustBillingOnComplete(_, _ string, req *dto.TaskRequest, result *dto.TaskResult, accepted channel.TaskPricing) channel.TaskPricing {
	result.BillingUsageSource = "admission_estimate"
	seconds, tokens := req.Seconds, accepted.CompletionTokens
	if result.Seconds > 0 {
		seconds = result.Seconds
	}
	if result.Usage != nil {
		tokens, result.BillingUsageSource = result.Usage.CompletionTokens, "upstream"
	}
	if len(accepted.ExprVars) > 0 {
		if result.Usage == nil {
			tokens = int64(accepted.ExprVars["completion_tokens"])
		}
		accepted.ExprVars["seconds"], accepted.ExprVars["completion_tokens"], accepted.ExprVars["total_tokens"] = float64(seconds), float64(tokens), float64(tokens)
		accepted.ExprVars["task_status"] = 0
		if result.Status == dto.TaskStatusSucceeded {
			accepted.ExprVars["task_status"] = 1
		}
		return accepted
	}
	if accepted.PerCallUSD > 0 {
		if result.Status != dto.TaskStatusSucceeded {
			return channel.TaskPricing{}
		}
		if result.Seconds > 0 {
			result.BillingUsageSource = "upstream"
		}
		accepted.Units = float64(seconds)
		return accepted
	}
	if result.Usage != nil {
		return channel.TaskPricing{Usage: result.Usage, CompletionTokens: tokens}
	}
	if result.Status != dto.TaskStatusSucceeded {
		return channel.TaskPricing{}
	}
	return accepted
}
