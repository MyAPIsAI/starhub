package dto

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGeminiNativePath(t *testing.T) {
	for _, method := range []string{"generateContent", "streamGenerateContent"} {
		model, stream, err := ParseGeminiPath("/v1beta/models/myapis-auto:" + method)
		if err != nil || model != "myapis-auto" || stream != (method == "streamGenerateContent") {
			t.Fatalf("path parsed as %q %t %v", model, stream, err)
		}
	}
	for _, path := range []string{"/v1beta/models/x:countTokens", "/v1beta/models/a/b:generateContent", "/v1beta/models/:generateContent", "/v1beta/models/x:generateContent?key=secret", "/v1/models/x:generateContent", "/v1beta/models/x:generateContent:extra"} {
		if _, _, err := ParseGeminiPath(path); err == nil {
			t.Fatalf("unsafe or unsupported path accepted: %s", path)
		}
	}
}

func TestGeminiRequestRoutingAndEstimates(t *testing.T) {
	body := []byte(`{"contents":[{"parts":[{"text":"12345678"}]}],"systemInstruction":{"parts":[{"text":"1234"}]},"generationConfig":{"maxOutputTokens":16,"thinkingConfig":{"thinkingBudget":8}},"tools":[{"functionDeclarations":[{"name":"lookup"}]}]}`)
	req, err := ParseGeminiRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	want := EstimateTokens("12345678") + EstimateTokens("1234") + EstimateTokens(string(req.Tools[0]))
	if req.InputTokens != want || !req.HasTools || !req.HasReasoning || req.Continuation || req.HasImages || *req.GenerationConfig.MaxOutputTokens != 16 || string(req.FirstUser) != `[{"text":"12345678"}]` {
		t.Fatalf("request summary incorrect: %+v, want tokens %d", req, want)
	}
	snake := strings.NewReplacer("systemInstruction", "system_instruction", "generationConfig", "generation_config", "maxOutputTokens", "max_output_tokens", "thinkingConfig", "thinking_config").Replace(string(body))
	other, err := ParseGeminiRequest([]byte(snake))
	if err != nil || other.InputTokens != want || *other.GenerationConfig.MaxOutputTokens != 16 || !other.HasReasoning {
		t.Fatalf("snake fields lost: %+v %v", other, err)
	}
}

func TestGeminiFunctionHistoryAndImages(t *testing.T) {
	image := strings.Repeat("A", 100000)
	body := `{"contents":[{"role":"user","parts":[{"text":"first"}]},{"role":"model","parts":[{"thoughtSignature":"opaque-state","functionCall":{"name":"lookup","args":{"q":"hello"}}}]},{"role":"user","parts":[{"functionResponse":{"name":"lookup","response":{"result":"done"},"parts":[{"inlineData":{"mimeType":"image/png","data":"` + image + `"}}]}}]}]}`
	req, err := ParseGeminiRequest([]byte(body))
	if err != nil || !req.HasTools || !req.HasReasoning || !req.HasImages || !req.Continuation || req.InputTokens > 100 || string(req.FirstUser) != `[{"text":"first"}]` {
		t.Fatalf("function history summary: %+v %v", req, err)
	}
	if !json.Valid(req.FirstUser) {
		t.Fatal("first user prefix is invalid JSON")
	}
	trimmed, err := ParseGeminiRequest([]byte(`{"contents":[{"role":"user","parts":[{"functionResponse":{"name":"lookup","response":{"result":"done"}}}]}]}`))
	if err != nil || !trimmed.Continuation || trimmed.FirstUser != nil {
		t.Fatalf("trimmed history got a fresh prefix: %+v %v", trimmed, err)
	}
}

func TestGeminiRequestRejectsUnsupportedAndMalformed(t *testing.T) {
	for _, body := range []string{
		`null`, `{}`, `{"contents":null}`, `{"contents":[{"role":"assistant","parts":[{"text":"x"}]}]}`,
		`{"contents":[{"parts":[]}]}`, `{"contents":[{"parts":[{}]}]}`, `{"contents":[{"parts":[{"text":12}]}]}`,
		`{"contents":[{"parts":[{"fileData":{"fileUri":"files/other-user"}}]}]}`,
		`{"contents":[{"parts":[{"inlineData":{"mimeType":"audio/wav","data":"AAAA"}}]}]}`,
		`{"contents":[{"parts":[{"text":"x"}]}],"cachedContent":"cachedContents/other-user"}`,
		`{"contents":[{"parts":[{"text":"x"}]}],"continuationToken":"state"}`,
		`{"contents":[{"parts":[{"text":"x"}]}],"tools":[{"googleSearch":{}}]}`,
		`{"contents":[{"parts":[{"text":"x"}]}],"tools":[null]}`,
		`{"contents":[{"parts":[{"text":"x"}]}],"generationConfig":{"maxOutputTokens":-1}}`,
		`{"contents":[{"parts":[{"text":"x"}]}],"generationConfig":{"maxOutputTokens":1.5}}`,
		`{"contents":[{"parts":[{"text":"x"}]}],"generationConfig":{"candidateCount":2}}`,
		`{"contents":[{"parts":[{"text":"x"}]}],"generation_config":{"response_modalities":["AUDIO"]}}`,
		`{"contents":[{"parts":[{"text":"x"}]}],"generationConfig":{},"generation_config":{}}`,
		`{"contents":[{"parts":[{"inlineData":{},"inline_data":{}}]}]}`,
	} {
		if req, err := ParseGeminiRequest([]byte(body)); err == nil {
			t.Fatalf("malformed or unsupported request accepted: %s => %+v", body, req)
		}
	}
}

func TestGeminiRequestBoundsNestedFunctionMedia(t *testing.T) {
	part := `{"text":"nested"}`
	for range 10 {
		part = `{"functionResponse":{"name":"nested","response":{},"parts":[` + part + `]}}`
	}
	if _, err := ParseGeminiRequest([]byte(`{"contents":[{"parts":[` + part + `]}]}`)); err == nil {
		t.Fatal("unbounded function nesting accepted")
	}
}
