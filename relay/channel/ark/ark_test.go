package ark

import (
	"encoding/json"
	"strings"
	"testing"

	"starhub/relay/channel"
	"starhub/relay/dto"
)

func TestArkEndpoints(t *testing.T) {
	a := Adaptor{}
	for _, base := range []string{"https://example.test/prefix", "https://example.test/prefix/api/v3/"} {
		if got := a.ChatURL(base, "", "", false); got != "https://example.test/prefix/api/v3/chat/completions" {
			t.Fatal(got)
		}
		for mode, path := range map[channel.RelayMode]string{channel.RelayModeImageGeneration: "/images/generations", channel.RelayModeEmbeddings: "/embeddings", channel.RelayModeResponses: "/responses"} {
			if got, err := a.ModeURL(base, mode); err != nil || got != "https://example.test/prefix/api/v3"+path {
				t.Fatalf("unexpected endpoint %s: %v", got, err)
			}
		}
	}
	if APIBaseURL("") != DefaultBaseURL+"/api/v3" {
		t.Fatal("missing default Ark endpoint")
	}
	for _, mode := range []channel.RelayMode{channel.RelayModeImageEdit, channel.RelayModeTTS, channel.RelayModeRerank} {
		if _, err := a.ModeURL("", mode); err != channel.ErrUnsupportedMode {
			t.Fatal("unsupported mode was advertised")
		}
	}
	if _, err := a.RealtimeURL("", ""); err != channel.ErrUnsupportedMode {
		t.Fatal("unsupported realtime was advertised")
	}
}

func TestArkVideoConversionAndValidation(t *testing.T) {
	a := VideoTaskAdaptor{}
	const model = "doubao-seedance-2-0-260128"
	var req dto.TaskRequest
	body := `{"model":"` + model + `","prompt":"a cat","seconds":5,"n":1,"extra":{"resolution":"720p","generate_audio":false,"content":[{"type":"image_url","image_url":{"url":"https://example.test/a.png"},"role":"first_frame"}]}}`
	if err := a.ValidateRequestAndSetAction([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	wire, err := a.ConvertSubmitRequest(&req)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(wire, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["model"] != model || payload["duration"] != float64(5) || payload["resolution"] != "720p" || payload["generate_audio"] != false || payload["n"] != nil || payload["max_output_tokens"] != nil || len(payload["content"].([]any)) != 2 {
		t.Fatalf("video request meaning changed: %s", wire)
	}
	if req.EstimatedInputTokens == nil || *req.EstimatedInputTokens != 0 || req.MaxOutputTokens < 100000 {
		t.Fatal("video reservation used text-token estimates")
	}
	for _, body := range []string{
		`{"model":"` + model + `","content":[{"type":"text","text":"a cat"}],"duration":5}`,
		`{"model":"` + model + `","content":[{"type":"image_url","image_url":{"url":"https://example.test/a.png"}}],"seconds":5}`,
		`{"model":"ep-fixture","prompt":"a cat","seconds":30,"size":"1280x720"}`,
		`{"model":"doubao-seedance-2-5-260628","prompt":"a cat","seconds":30,"resolution":"4k"}`,
		`{"model":"doubao-seedance-1-0-pro-fast-251015","prompt":"a cat","seconds":5,"resolution":"1080p"}`,
	} {
		if err := a.ValidateRequestAndSetAction([]byte(body), &req); err != nil {
			t.Fatalf("valid request rejected: %s: %v", body, err)
		}
		if _, err := a.ConvertSubmitRequest(&req); err != nil {
			t.Fatal(err)
		}
	}
	for _, addition := range []string{
		`"n":2`, `"seconds":0`, `"seconds":-1`, `"seconds":16`, `"seconds":3`, `"seconds":null`,
		`"frames":100`, `"callback_url":"https://example.test/callback"`, `"stream":true`,
		`"estimated_input_tokens":-1`,
		`"extra":{"model":"other"}`, `"extra":{"n":2}`, `"extra":{"duration":100}`,
		`"seconds":5,"duration":5`, `"ratio":null`, `"ratio":"other"`,
		`"size":"1024x1024"`, `"resolution":"8k"`, `"resolution":"720p","size":"1280x720"`,
		`"generate_audio":"false"`, `"seed":1.5`, `"seed":-2`,
		`"content":[]`, `"content":null`, `"content":[{"type":"draft_task","draft_task":{"id":"other"}}]`,
		`"content":[{"type":"image_url","image_url":{"url":"file:///private"}}]`,
		`"content":[{"type":"text","text":"cat --dur 60"}]`,
	} {
		body := `{"model":"` + model + `","prompt":"a cat",` + addition + `}`
		if err := a.ValidateRequestAndSetAction([]byte(body), &req); err == nil {
			t.Fatalf("unsafe or ignored parameter accepted: %s", body)
		}
	}
	if err := a.ValidateRequestAndSetAction([]byte(`{"model":"doubao-seedance-2-0-fast-260128","prompt":"cat","resolution":"1080p"}`), &req); err == nil {
		t.Fatal("unsupported model resolution accepted")
	}
	for _, base := range []string{"https://example.test/prefix", "https://example.test/prefix/api/v3/"} {
		if a.SubmitURL(base) != "https://example.test/prefix/api/v3/contents/generations/tasks" || !strings.HasSuffix(a.FetchURL(base, "cgt-fixture"), "/tasks/cgt-fixture") {
			t.Fatal("incorrect video endpoint")
		}
	}
}

func TestArkVideoResultAndBilling(t *testing.T) {
	a := VideoTaskAdaptor{}
	if id, err := a.ParseSubmitResponse(200, []byte(`{"id":"cgt-fixture"}`)); err != nil || id != "cgt-fixture" {
		t.Fatalf("submit response: %s %v", id, err)
	}
	for _, body := range []string{`{}`, `{"id":"../other"}`, `{"id":"cgt-fixture","error":{"code":"failed"}}`} {
		if _, err := a.ParseSubmitResponse(200, []byte(body)); err == nil {
			t.Fatal("invalid submit response accepted")
		}
	}
	for status, want := range map[string]string{"queued": dto.TaskStatusSubmitted, "running": dto.TaskStatusInProgress, "failed": dto.TaskStatusFailed, "expired": dto.TaskStatusFailed, "cancelled": dto.TaskStatusCancelled} {
		result, err := a.ParseTaskResult(200, []byte(`{"status":"`+status+`"}`))
		if err != nil || result.Status != want {
			t.Fatalf("status %s: %+v %v", status, result, err)
		}
	}
	const success = `{"status":"succeeded","duration":5,"content":{"video_url":"https://example.test/video.mp4?signature=fixture","last_frame_url":"https://example.test/frame.png"},"usage":{"completion_tokens":123,"total_tokens":123}}`
	result, err := a.ParseTaskResult(200, []byte(success))
	if err != nil || result.Usage.PromptTokens != 0 || result.Usage.CompletionTokens != 123 || len(result.Outputs) != 1 || !strings.Contains(result.Outputs[0], "signature=fixture") {
		t.Fatalf("invalid video result: %+v %v", result, err)
	}
	for _, body := range []string{`{"status":"unknown"}`, `{"status":"succeeded"}`, strings.Replace(success, "https://example.test/video.mp4?signature=fixture", "javascript:alert(1)", 1), strings.Replace(success, `"total_tokens":123`, `"total_tokens":124`, 1), strings.Replace(success, `"completion_tokens":123`, `"completion_tokens":-1`, 1), strings.Replace(success, `"duration":5`, `"duration":31`, 1), strings.Replace(success, `"completion_tokens":123`, `"completion_tokens":"123"`, 1)} {
		if _, err := a.ParseTaskResult(200, []byte(body)); err == nil {
			t.Fatalf("invalid result accepted: %s", body)
		}
	}
	req := &dto.TaskRequest{Seconds: 6, N: 1, MaxOutputTokens: 1000}
	for _, options := range []channel.TaskPricingOptions{{}, {ModelPriceSet: true, ModelPriceUSD: 0.1}, {Expression: "seconds"}} {
		accepted := a.EstimateBillingWithOptions("model", "default", req, options)
		actual := a.AdjustBillingOnComplete("model", "default", req, result, accepted)
		if options.ModelPriceSet && (actual.Units != 5 || actual.PerCallUSD != 0.1) {
			t.Fatal("delivered duration not used")
		}
		if options.Expression != "" && (actual.ExprVars["seconds"] != 5 || actual.ExprVars["completion_tokens"] != 123) {
			t.Fatal("actual expression facts lost")
		}
		if options.Expression == "" && !options.ModelPriceSet && actual.CompletionTokens != 123 {
			t.Fatal("actual video tokens lost")
		}
		cancel := a.AdjustBillingOnComplete("model", "default", req, &dto.TaskResult{Status: dto.TaskStatusCancelled}, accepted)
		if cancel.PerCallUSD != 0 || cancel.CompletionTokens != 0 || (len(cancel.ExprVars) > 0 && cancel.ExprVars["task_status"] != 0) {
			t.Fatal("Sora cancellation discounts leaked into Ark")
		}
	}
	if free := a.EstimateBillingWithOptions("model", "default", req, channel.TaskPricingOptions{ModelPriceSet: true}); free.CompletionTokens != 0 || free.PerCallUSD != 0 {
		t.Fatal("explicit free price ignored")
	}
	for _, usage := range []string{`{"total_tokens":0}`, `{"completion_tokens":0}`, `{"total_tokens":123}`} {
		body := `{"status":"succeeded","content":{"video_url":"https://example.test/video.mp4"},"usage":` + usage + `}`
		result, err := a.ParseTaskResult(200, []byte(body))
		if err != nil || result.Usage == nil {
			t.Fatalf("explicit usage was lost: %v", err)
		}
	}
}
