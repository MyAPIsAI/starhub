package service

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type aliyunSMSResponseTransport struct {
	status int
	body   string
}

func (tr aliyunSMSResponseTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: tr.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(tr.body)),
	}, nil
}

func TestAliyunSMSResponseClassification(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"success", 200, `{"Code":"OK"}`, nil},
		{"business limit on success status", 200, `{"Code":"isv.BUSINESS_LIMIT_CONTROL"}`, ErrSMSRateLimited},
		{"business limit on bad request", 400, `{"Code":"isv.BUSINESS_LIMIT_CONTROL"}`, ErrSMSRateLimited},
		{"business limit on forbidden", 403, `{"Code":"isv.BUSINESS_LIMIT_CONTROL"}`, ErrSMSRateLimited},
		{"throttling on unavailable", 503, `{"Code":"Throttling"}`, ErrSMSRateLimited},
		{"user throttling on bad request", 400, `{"Code":"Throttling.User"}`, ErrSMSRateLimited},
		{"HTTP throttling without JSON", 429, "Too Many Requests", ErrSMSRateLimited},
		{"server error with OK code", 500, `{"Code":"OK"}`, ErrSMSSendFailed},
		{"bad request with OK code", 400, `{"Code":"OK"}`, ErrSMSSendFailed},
		{"redirect with OK code", 302, `{"Code":"OK"}`, ErrSMSSendFailed},
		{"other business error", 200, `{"Code":"isv.AMOUNT_NOT_ENOUGH"}`, ErrSMSSendFailed},
		{"other HTTP error", 400, `{"Code":"InvalidParameter"}`, ErrSMSSendFailed},
		{"malformed success response", 200, "not JSON", ErrSMSSendFailed},
		{"malformed error response", 503, "not JSON", ErrSMSSendFailed},
		{"empty response", 200, "", ErrSMSSendFailed},
		{"missing business code", 200, `{}`, ErrSMSSendFailed},
		{"oversized business code", 200, `{"Code":"` + strings.Repeat("x", 16<<10) + `"}`, ErrSMSSendFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &aliyunSMSProvider{
				accessKeyID: "FixtureAccessID", secret: "FixtureSecret", signName: "Fixture Sign", templateCode: "SMS_123456",
				client: &http.Client{Transport: aliyunSMSResponseTransport{status: tc.status, body: tc.body}},
			}
			if err := provider.Send("+8613812345678", "123456"); !errors.Is(err, tc.want) {
				t.Fatalf("Send() = %v, want %v", err, tc.want)
			}
		})
	}
}
