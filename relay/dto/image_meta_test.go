package dto

import (
	"encoding/json"
	"errors"
	"testing"
)

// walkCount is the WalkImageResponse equivalent of ImageResponse.Count.
func walkCount(body []byte) (int, *ImageUsage, error) {
	urls, b64s, usage, err := WalkImageResponse(body, nil)
	if err != nil {
		return 0, nil, err
	}
	return max(urls, b64s), usage, nil
}

// TestWalkImageResponseCountParity walks the same bodies through the
// legacy materializing parse and the streaming walker: counts, usage and
// error/no-error must agree on every shape (P7.1 replaced the hot path,
// not the billing semantics).
func TestWalkImageResponseCountParity(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"single b64", `{"data":[{"b64_json":"QUJD"}]}`},
		{"single url", `{"data":[{"url":"https://x/y.png"}]}`},
		{"two b64", `{"data":[{"b64_json":"A"},{"b64_json":"B"}]}`},
		{"split url+b64 entries", `{"data":[{"url":"https://x/y.png"},{"b64_json":"QUJD"}]}`},
		{"one entry with both", `{"data":[{"url":"https://x/y.png","b64_json":"QUJD"}]}`},
		{"payload-less entries", `{"data":[{"revised_prompt":"hi"},{"b64_json":""}]}`},
		{"null values", `{"data":[{"b64_json":null},{"url":null}]}`},
		{"missing data key", `{"created":1}`},
		{"data null", `{"data":null,"usage":{"input_tokens":1}}`},
		{"usage present", `{"data":[{"b64_json":"QUJD"}],"usage":{"input_tokens":12,"output_tokens":34,"total_tokens":46}}`},
		{"usage before data", `{"usage":{"input_tokens":12},"data":[{"b64_json":"QUJD"}]}`},
		{"usage null", `{"data":[{"b64_json":"QUJD"}],"usage":null}`},
		{"extra fields skipped", `{"created":123,"data":[{"b64_json":"QUJD","revised_prompt":"p"}],"usage":{"input_tokens":5}}`},
		{"not an object", `[]`},
		{"data not an array", `{"data":{}}`},
		{"entry not an object", `{"data":["str"]}`},
		{"b64 not a string", `{"data":[{"b64_json":42}]}`},
		{"url not a string", `{"data":[{"url":true}]}`},
		{"malformed json", `{"data":[{"b64_json":`},
		{"trailing garbage", `{"data":[]} {`},
		{"duplicate data key", `{"data":[{"b64_json":"A"}],"data":[{"b64_json":"B"}]}`},
		{"created type error", `{"created":"x","data":[{"b64_json":"A"}]}`},
		{"nested garbage entries", `{"data":[{"x":{"y":[1,2,{"z":"}]"}},"b64_json":"A"}]}`},
		{"escaped payload content", `{"data":[{"b64_json":"a\\"b"}]}`},
		{"spaces inside quotes count", `{"data":[{"b64_json":"  "}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)

			legacyCount, legacyUsage, legacyErr := func() (int, *ImageUsage, error) {
				parsed, err := ParseImageResponse(body)
				if err != nil {
					return 0, nil, err
				}
				return parsed.Count(), parsed.Usage, nil
			}()

			walkedCount, walkedUsage, walkedErr := walkCount(body)

			if (legacyErr == nil) != (walkedErr == nil) {
				t.Fatalf("error parity: legacy err=%v, walk err=%v", legacyErr, walkedErr)
			}
			if legacyErr != nil {
				return // both reject; both end at count 0 in the callers
			}
			if legacyCount != walkedCount {
				t.Fatalf("count parity: legacy=%d walk=%d", legacyCount, walkedCount)
			}
			if (legacyUsage == nil) != (walkedUsage == nil) {
				t.Fatalf("usage parity: legacy=%v walk=%v", legacyUsage, walkedUsage)
			}
			if legacyUsage != nil && *legacyUsage != *walkedUsage {
				t.Fatalf("usage value parity: legacy=%+v walk=%+v", *legacyUsage, *walkedUsage)
			}
		})
	}
}

// TestWalkImageResponseEmitOrder verifies the callback receives every
// entry and that the raw values carry the original JSON string literals.
func TestWalkImageResponseEmitOrder(t *testing.T) {
	body := []byte(`{"data":[{"b64_json":"AA","url":"u1"},{"b64_json":"BB"},{"url":"u2"}]}`)
	var got []string
	_, _, _, err := WalkImageResponse(body, func(e ImageEntryRaw) error {
		got = append(got, string(RawStringLiteral(e.B64))+"|"+string(RawStringLiteral(e.URL)))
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	want := []string{`"AA"|"u1"`, `"BB"|""`, `""|"u2"`}
	if len(got) != len(want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entry %d = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestWalkImageResponseEmitError(t *testing.T) {
	sentinel := errors.New("stop")
	_, _, _, err := WalkImageResponse([]byte(`{"data":[{"b64_json":"A"},{"b64_json":"B"}]}`),
		func(e ImageEntryRaw) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("emit error propagation: %v", err)
	}
}

// TestWalkImageResponseDuplicateDataKeyLastWins: both the legacy
// json.Unmarshal and the walker keep the LAST duplicate key.
func TestWalkImageResponseDuplicateDataKeyLastWins(t *testing.T) {
	count, _, err := walkCount([]byte(`{"data":[{"b64_json":"A"},{"b64_json":"B"}],"data":[{"b64_json":"C"}]}`))
	if err != nil || count != 1 {
		t.Fatalf("duplicate data key: count=%d err=%v, want 1 (last wins)", count, err)
	}
}

// TestWalkImageResponseScannerShapes covers the in-place entry scanner
// on shapes the parity table exercises only end-to-end.
func TestWalkImageResponseScannerShapes(t *testing.T) {
	// Duplicate keys inside one entry: last occurrence wins.
	count, _, err := walkCount([]byte(`{"data":[{"b64_json":"A","b64_json":"B"}]}`))
	if err != nil || count != 1 {
		t.Fatalf("dup entry keys: count=%d err=%v", count, err)
	}
	// Whitespace-heavy layout: one b64 entry + one url entry -> max(1,1)=1.
	count, _, err = walkCount([]byte("{\n  \"data\" : [\n    { \"b64_json\" : \"QUJD\" } ,\n    { \"url\" : \"https://x\" }\n  ]\n}"))
	if err != nil || count != 1 {
		t.Fatalf("whitespace layout: count=%d err=%v", count, err)
	}
	// Empty entry objects are payload-less.
	count, _, err = walkCount([]byte(`{"data":[{},{ }]}`))
	if err != nil || count != 0 {
		t.Fatalf("empty entries: count=%d err=%v", count, err)
	}
	// Entry not an object.
	if _, _, err := walkCount([]byte(`{"data":["str"]}`)); err == nil {
		t.Fatalf("non-object entry must error")
	}
	// b64_json value not a string.
	if _, _, err := walkCount([]byte(`{"data":[{"b64_json":42}]}`)); err == nil {
		t.Fatalf("non-string b64_json must error")
	}
}

func TestRawStringLiteral(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{``, `""`},
		{`null`, `""`},
		{`""`, `""`},
		// Spaces between the quotes are payload content: pass through
		// verbatim (legacy re-marshal emitted the two-space string too).
		{`"  "`, `"  "`},
		{`"abc"`, `"abc"`},
		{`"quoted \"x\""`, `"quoted \"x\""`},
	}
	for _, tc := range cases {
		if got := string(RawStringLiteral(json.RawMessage(tc.raw))); got != tc.want {
			t.Fatalf("RawStringLiteral(%s) = %s, want %s", tc.raw, got, tc.want)
		}
	}
}

// TestParseImageStreamEventMetaParity checks the meta parser against the
// legacy parser for every billing field.
func TestParseImageStreamEventMetaParity(t *testing.T) {
	cases := []struct {
		name      string
		eventName string
		payload   string
	}{
		{"completed", "image_generation.completed", `{"type":"image_generation.completed","output_index":3,"b64_json":"QUJD"}`},
		{"partial", "image_generation.partial_image", `{"type":"image_generation.partial_image","output_index":0,"b64_json":"cGFydGlhbA=="}`},
		{"type from event name", "", `{"output_index":1}`},
		{"usage event", "", `{"type":"image_generation.completed","usage":{"input_tokens":7,"output_tokens":9,"total_tokens":16}}`},
		{"error event", "error", `{"type":"error","error":{"code":"x","message":"boom"}}`},
		{"invalid json", "", `not json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			legacy, legacyErr := ParseImageStreamEvent([]byte(tc.eventName), []byte(tc.payload))
			meta, metaErr := ParseImageStreamEventMeta([]byte(tc.eventName), []byte(tc.payload))
			if (legacyErr == nil) != (metaErr == nil) {
				t.Fatalf("error parity: legacy=%v meta=%v", legacyErr, metaErr)
			}
			if legacyErr != nil {
				return
			}
			if legacy.Type != meta.Type || legacy.OutputIndex != meta.OutputIndex ||
				legacy.ErrorMessage != meta.ErrorMessage {
				t.Fatalf("field parity: legacy=%+v meta=%+v", legacy, meta)
			}
			if (legacy.Usage == nil) != (meta.Usage == nil) {
				t.Fatalf("usage parity: legacy=%v meta=%v", legacy.Usage, meta.Usage)
			}
			if legacy.Usage != nil && *legacy.Usage != *meta.Usage {
				t.Fatalf("usage value parity: legacy=%+v meta=%+v", *legacy.Usage, *meta.Usage)
			}
		})
	}
}
