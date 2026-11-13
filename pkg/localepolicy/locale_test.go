package localepolicy

import "testing"

func TestResolveAccountBrowserWeightsAndRegionDefault(t *testing.T) {
	for _, tc := range []struct{ account, header, fallback, want string }{{"ar", "en-US", "zh", "ar"}, {"fr", "ko-KR,en;q=0.8", "ar", "ko"}, {"", "ar;q=0.1,en;q=0.9", "zh", "en"}, {"", "en;q=0,ar;q=0", "zh", "zh"}, {"", "unsupported,english", "ar", "ar"}, {"", "*;q=0.7", "ar", "ar"}, {"", "en;q=NaN,ar;q=0.5", "zh", "ar"}} {
		if got := Resolve(tc.account, tc.header, tc.fallback, []string{"zh", "en", "ar", "ko"}); got != tc.want {
			t.Fatalf("locale %+v: %s", tc, got)
		}
	}
	if got := Resolve("zh", "ko,en;q=0.8", "ar", []string{"ar", "en"}); got != "en" {
		t.Fatalf("unsupported account preference escaped region policy: %s", got)
	}
}
