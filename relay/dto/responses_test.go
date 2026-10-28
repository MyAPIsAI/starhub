package dto

import "testing"

func TestResponsesUsageNormalizesTotalsAndSubsets(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		body := `{"id":"resp_review","usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":30},"output_tokens":20,"output_tokens_details":{"reasoning_tokens":12},"total_tokens":120,"RequestProtocol":"forged"}}`
		if wrapped {
			body = `{"type":"response.completed","response":` + body + `}`
		}
		usage, err := ParseResponsesUsage([]byte(body))
		if err != nil || usage == nil {
			t.Fatalf("usage=%+v err=%v", usage, err)
		}
		if usage.PromptTokens != 100 || usage.CompletionTokens != 20 || usage.TotalTokens() != 120 || usage.CachedTokens() != 30 || usage.CompletionTokensDetails.ReasoningTokens != 12 || usage.RequestProtocol != "" {
			t.Fatalf("usage totals/subsets were added twice or protocol was trusted: %+v", usage)
		}
	}
}

func TestResponsesUsagePreservesMissingVersusZero(t *testing.T) {
	for _, body := range []string{`{"id":"resp_review"}`, `{"usage":null}`, `{"type":"response.created","response":{"usage":null}}`, `{"type":"response.output_text.delta","delta":"Hello"}`} {
		usage, err := ParseResponsesUsage([]byte(body))
		if err != nil || usage != nil {
			t.Fatalf("missing report was treated as free usage: %s %+v %v", body, usage, err)
		}
	}
	usage, err := ParseResponsesUsage([]byte(`{"usage":{"input_tokens":0,"output_tokens":0}}`))
	if err != nil || usage == nil || usage.TotalTokens() != 0 {
		t.Fatalf("explicit zero report lost: %+v %v", usage, err)
	}
}

func TestResponsesUsageRejectsInvalidTokenReports(t *testing.T) {
	for _, body := range []string{
		`null`, `[]`, `true`, `{`,
		`{"usage":{}}`,
		`{"usage":{"input_tokens":1}}`,
		`{"usage":{"input_tokens":1.5,"output_tokens":1}}`,
		`{"usage":{"input_tokens":"100","output_tokens":1}}`,
		`{"usage":{"input_tokens":-1,"output_tokens":1}}`,
		`{"usage":{"input_tokens":1,"output_tokens":-1}}`,
		`{"usage":{"input_tokens":9223372036854775807,"output_tokens":1}}`,
		`{"type":"response.completed","response":[]}`,
	} {
		if usage, err := ParseResponsesUsage([]byte(body)); err == nil || usage != nil {
			t.Fatalf("invalid report accepted: %s %+v %v", body, usage, err)
		}
	}
}

func TestResponsesUsageClampsDetailsAndTrustsEventResponse(t *testing.T) {
	for _, negative := range []bool{false, true} {
		detail := "999"
		wantInput, wantOutput := int64(100), int64(20)
		if negative {
			detail = "-1"
			wantInput, wantOutput = 0, 0
		}
		body := `{"usage":{"input_tokens":9999,"output_tokens":9999},"response":{"usage":{"input_tokens":100,"output_tokens":20,"input_tokens_details":{"cached_tokens":` + detail + `},"output_tokens_details":{"reasoning_tokens":` + detail + `}}}}`
		usage, err := ParseResponsesUsage([]byte(body))
		if err != nil || usage == nil {
			t.Fatalf("usage=%+v err=%v", usage, err)
		}
		if usage.TotalTokens() != 120 || usage.CachedTokens() != wantInput || usage.CompletionTokensDetails.ReasoningTokens != wantOutput {
			t.Fatalf("unbounded details or wrong event report: %+v", usage)
		}
	}
}
