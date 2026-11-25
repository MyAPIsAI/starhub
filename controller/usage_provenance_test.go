package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"starhub/metric"
	"starhub/relay"
	"starhub/relay/dto"
)

func TestUsageProvenanceCannotBeSuppliedByProvider(t *testing.T) {
	var usage relay.Usage
	if err := json.Unmarshal([]byte(`{"prompt_tokens":3,"UsageSource":"upstream","UpstreamRequestID":"forged"}`), &usage); err != nil {
		t.Fatal(err)
	}
	if usage.UsageSource != "" || usage.UpstreamRequestID != "" {
		t.Fatal("untrusted provenance accepted")
	}
	span := metric.BeginRequest(metric.RequestMeta{})
	a := span.BeginAttempt(1)
	a.RecordHeaders("provider-actual-id")
	flow := &relayFlow{ctx: context.Background(), attempt: a}
	flow.annotateUsage(&usage)
	if usage.UsageSource != "unknown" || usage.UpstreamRequestID != "provider-actual-id" {
		t.Fatalf("wrong source: %+v", usage)
	}
	estimate := relay.EstimateIRUsage(&dto.ChatRequest{}, "text")
	flow.annotateUsage(estimate)
	if estimate.UsageSource != "estimated" {
		t.Fatal("estimate relabeled as provider usage")
	}
	response, err := dto.ParseOpenAIChatResponse([]byte(`{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`))
	if err != nil || response.Usage.UsageSource != "upstream" {
		t.Fatalf("provider report lost: %v", err)
	}
	span.EndAttempt(a, metric.AttemptSuccess, 200)
	span.EndRequest(200)
	for _, id := range []string{strings.Repeat("a", 65), "bad id"} {
		resp := &http.Response{Header: make(http.Header)}
		resp.Header.Set("X-Request-Id", id)
		if upstreamRequestID(resp) != "" {
			t.Fatal("unsafe provider identifier accepted")
		}
	}
}
