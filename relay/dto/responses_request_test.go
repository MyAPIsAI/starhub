package dto

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResponsesRequestReadsTextToolsAndCapabilities(t *testing.T) {
	input := strings.Repeat("a", 1000)
	body := `{"model":"myapis-auto","instructions":"review","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}],"max_output_tokens":64,"input":[{"role":"user","content":"first message"},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":` + quoteResponsesString(input) + `},{"type":"function_call_output","call_id":"call_1","output":[{"type":"input_text","text":"result"},{"type":"input_image","image_url":"data:image/png;base64,AA=="}]}]}`
	req, err := ParseResponsesRequest([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if req.InputTokens < EstimateTokens(input) || req.FirstUser == nil || string(req.FirstUser) != `"first message"` || !req.HasTools || !req.HasImages || req.HasFiles || !req.Continuation || req.MaxOutputTokens == nil || *req.MaxOutputTokens != 64 {
		t.Fatalf("request summary lost billing/routing constraints: %+v", req)
	}
}

func TestResponsesRequestDoesNotCountImageBytesAsText(t *testing.T) {
	body := `{"model":"m","input":[{"role":"user","content":[{"type":"input_text","text":"hello"},{"type":"input_image","image_url":"data:image/png;base64,` + strings.Repeat("A", 100000) + `"}]}]}`
	req, err := ParseResponsesRequest([]byte(body))
	if err != nil || req.InputTokens != EstimateTokens("hello") || !req.HasImages {
		t.Fatalf("image bytes became text tokens: %+v %v", req, err)
	}
}

func TestResponsesRequestCountsCustomToolsAndOpaqueHistory(t *testing.T) {
	for _, item := range []string{
		`{"type":"custom_tool_call","call_id":"call_1","name":"apply_patch","input":` + quoteResponsesString(strings.Repeat("x", 1000)) + `}`,
		`{"type":"custom_tool_call_output","call_id":"call_1","output":` + quoteResponsesString(strings.Repeat("x", 1000)) + `}`,
		`{"type":"reasoning","summary":[{"type":"summary_text","text":"review"}],"encrypted_content":` + quoteResponsesString(strings.Repeat("x", 1000)) + `}`,
		`{"type":"compaction","encrypted_content":` + quoteResponsesString(strings.Repeat("x", 1000)) + `}`,
	} {
		req, err := ParseResponsesRequest([]byte(`{"model":"m","input":[` + item + `]}`))
		if err != nil || req.InputTokens < EstimateTokens(strings.Repeat("x", 1000)) || !req.Continuation || req.FirstUser != nil {
			t.Fatalf("tool/state history was free or inferred a new prefix: %+v %v", req, err)
		}
	}
}

func TestResponsesRequestPreservesInputFormsAndReferences(t *testing.T) {
	for _, body := range []string{
		`{"model":"m","input":"hello"}`,
		`{"model":"m","input":[{"role":"developer","content":"review"},{"role":"user","content":"hello"}]}`,
		`{"model":"m","previous_response_id":"resp_review","input":"hello"}`,
		`{"model":"m","background":true,"conversation":"conv_review","input":"hello"}`,
	} {
		req, err := ParseResponsesRequest([]byte(body))
		if err != nil || req.FirstUser == nil || req.InputTokens == 0 {
			t.Fatalf("valid request lost: %+v %v", req, err)
		}
	}
	for _, content := range []string{`[{"type":"input_file","file_data":"AA=="}]`, `[{"type":"input_image","file_id":"file_review"}]`} {
		req, err := ParseResponsesRequest([]byte(`{"model":"m","input":[{"role":"user","content":` + content + `}]}`))
		if err != nil || !req.HasFiles {
			t.Fatalf("provider file reference was not identified: %+v %v", req, err)
		}
	}
}

func TestResponsesRequestRejectsMalformedBillingAndHistory(t *testing.T) {
	for _, body := range []string{
		`null`, `[]`, `{}`, `{"model":"m","input":null}`, `{"model":"m","input":{}}`,
		`{"model":"m","max_output_tokens":-1}`, `{"model":"m","max_output_tokens":0}`, `{"model":"m","max_output_tokens":1.5}`,
		`{"model":"m","input":[null]}`, `{"model":"m","input":[{"role":"tool","content":"result"}]}`,
		`{"model":"m","input":[{"type":"item_reference","id":"item_review"}]}`,
		`{"model":"m","input":[{"role":"user","content":[{"type":"unknown"}]}]}`,
	} {
		if req, err := ParseResponsesRequest([]byte(body)); err == nil || req != nil {
			t.Fatalf("malformed request admitted: %s %+v %v", body, req, err)
		}
	}
}

func quoteResponsesString(value string) string { raw, _ := json.Marshal(value); return string(raw) }
