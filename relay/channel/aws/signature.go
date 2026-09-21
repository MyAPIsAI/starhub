package aws

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// SigV4 signs a request for an AWS service (P3.1's "signature V4"
// representative). The canonical request covers the exact method / path /
// query / headers / body bytes that go on the wire, so signing MUST happen
// after the request is fully built.
type signer struct {
	region    string
	service   string
	accessKey string
	secretKey string
}

// ParseChannelKey splits a channel key of the form
// "accessKeyId|secretAccessKey|region" (the secret charset never contains
// the pipe separator).
func ParseChannelKey(key string) (accessKey, secretKey, region string, err error) {
	parts := strings.Split(key, "|")
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("aws channel key must be \"accessKeyId|secretAccessKey|region\"")
	}
	return parts[0], parts[1], parts[2], nil
}

const (
	amzDateFormat = "20060102T150405Z"
	dateFormat    = "20060102"
	algorithm     = "AWS4-HMAC-SHA256"
)

// sign computes the Authorization header value and stamps the x-amz-date
// header onto the request.
func (s signer) sign(req *http.Request, body []byte, now time.Time) error {
	amzDate := now.UTC().Format(amzDateFormat)
	dateStamp := now.UTC().Format(dateFormat)

	payloadHash := sha256Hex(body)

	req.Header.Set("X-Amz-Date", amzDate)

	signedHeaders, canonicalHeaders := canonicalHeaders(req)

	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQuery := canonicalQueryString(req)

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := strings.Join([]string{dateStamp, s.region, s.service, "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		algorithm,
		amzDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := deriveKey(s.secretKey, dateStamp, s.region, s.service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, s.accessKey, scope, signedHeaders, signature))
	return nil
}

func canonicalHeaders(req *http.Request) (signedHeaders, canonical string) {
	names := make([]string, 0, 8)
	values := map[string]string{}
	host := req.URL.Host
	if host == "" {
		host = req.Host
	}
	names = append(names, "host")
	values["host"] = host
	for name, vals := range req.Header {
		lower := strings.ToLower(name)
		switch lower {
		case "host", "content-length":
			continue
		}
		if !isSignedHeader(lower) {
			continue
		}
		if _, seen := values[lower]; !seen {
			names = append(names, lower)
		}
		values[lower] = strings.TrimSpace(strings.Join(vals, ","))
	}
	sort.Strings(names)
	var sb strings.Builder
	for _, name := range names {
		sb.WriteString(name)
		sb.WriteString(":")
		sb.WriteString(values[name])
		sb.WriteString("\n")
	}
	return strings.Join(names, ";"), sb.String()
}

// isSignedHeader restricts the signature to the headers the AWS SigV4
// canonical form typically covers (everything the request sets except
// hop-by-hop and user-agent noise).
func isSignedHeader(lower string) bool {
	switch lower {
	case "authorization", "user-agent", "expect", "connection", "transfer-encoding",
		"content-length", "accept-encoding":
		return false
	}
	return true
}

func canonicalQueryString(req *http.Request) string {
	if len(req.URL.RawQuery) == 0 {
		return ""
	}
	pairs := strings.Split(req.URL.RawQuery, "&")
	sort.Strings(pairs)
	return strings.Join(pairs, "&")
}

func deriveKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
