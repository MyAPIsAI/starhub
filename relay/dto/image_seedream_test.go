package dto

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestSeedream45Sizes(t *testing.T) {
	for _, tc := range []struct {
		size string
		ok   bool
	}{
		{"", true}, {"2K", true}, {"4K", true},
		{"2048x2048", true}, {"4096x4096", true}, {"1920x1920", true},
		{"7680x480", true}, {"480x7680", true},
		{"1K", false}, {"3K", false}, {"auto", false}, {"2k", false},
		{"1024x1024", false}, {"4096x4097", false}, {"0x4096", false},
		{"8192x480", false}, {"480x8192", false}, {"2048×2048", false},
		{"4294967296x4294967296", false}, {"4294967295x4294967295", false},
	} {
		t.Run(tc.size, func(t *testing.T) {
			body := fmt.Appendf(nil, `{"model":"doubao-seedream-4-5-251128","prompt":"a cat","size":%q,"watermark":false,"sequential_image_generation":"disabled"}`, tc.size)
			req, err := ParseImageRequest(body)
			if (err == nil) != tc.ok {
				t.Fatalf("size %q: error = %v, want valid %v", tc.size, err, tc.ok)
			}
			if err != nil {
				return
			}
			out, err := req.MarshalBody()
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatal(err)
			}
			if req.Size != tc.size || got["watermark"] != false || got["sequential_image_generation"] != "disabled" {
				t.Fatal("provider parameters changed during parsing")
			}
		})
	}
	for _, model := range []string{"dall-e-3", "gpt-image-1", "other-image"} {
		if err := (&ImageRequest{Model: model, Size: "2K"}).ValidateSize(); err == nil {
			t.Fatalf("Seedream preset unexpectedly accepted for %s", model)
		}
	}
}
