package aws

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestSigV4AgainstAWSDocumentedVector reproduces the worked example of the
// AWS "Signature Version 4 signing process" documentation (the
// ListUsers IAM request). If this test breaks, the signer drifted from
// the specification — not from the test.
func TestSigV4AgainstAWSDocumentedVector(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet,
		"https://iam.amazonaws.com/?Action=ListUsers&Version=2010-05-08", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")

	s := signer{
		region:    "us-east-1",
		service:   "iam",
		accessKey: "AKIDEXAMPLE",
		secretKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
	}
	when := time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC)
	if err := s.sign(req, nil, when); err != nil {
		t.Fatalf("sign: %v", err)
	}

	want := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/iam/aws4_request, " +
		"SignedHeaders=content-type;host;x-amz-date, " +
		"Signature=5d672d79c15b13162d9279b0855cfba6789a8edb4c82c400e06b5924a6f2b5d7"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("Authorization mismatch:\n got: %s\nwant: %s", got, want)
	}
	if req.Header.Get("X-Amz-Date") != "20150830T123600Z" {
		t.Fatalf("x-amz-date = %q", req.Header.Get("X-Amz-Date"))
	}
}

func TestParseChannelKey(t *testing.T) {
	ak, sk, region, err := ParseChannelKey("AKIA123|secretsecret|eu-west-1")
	if err != nil || ak != "AKIA123" || sk != "secretsecret" || region != "eu-west-1" {
		t.Fatalf("parse: %v %s %s %s", err, ak, sk, region)
	}
	for _, bad := range []string{"", "only-two|parts", "a|b", "a|b|"} {
		if _, _, _, err := ParseChannelKey(bad); err == nil {
			t.Fatalf("key %q must be rejected", bad)
		}
	}
}

func TestBedrockChatURLAndRegion(t *testing.T) {
	a := Adaptor{}
	key := "AK|x|eu-central-1"
	if got := a.ChatURL("", key, "anthropic.claude-3-5-sonnet", false); got !=
		"https://bedrock-runtime.eu-central-1.amazonaws.com/model/anthropic.claude-3-5-sonnet/invoke" {
		t.Fatalf("invoke url = %q", got)
	}
	if got := a.ChatURL("", key, "m", true); !strings.HasSuffix(got, "/invoke-with-response-stream") {
		t.Fatalf("stream url = %q", got)
	}
	// an explicit base_url wins over the key region
	if got := a.ChatURL("https://proxy.internal", key, "m", false); got !=
		"https://proxy.internal/model/m/invoke" {
		t.Fatalf("proxy url = %q", got)
	}
}
