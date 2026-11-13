package model

import "testing"

func TestRegionalPhoneNormalizationUsesCountryMetadata(t *testing.T) {
	for _, tc := range []struct{ country, input, want string }{{"CN", "13812345678", "+8613812345678"}, {"AE", "050 123 4567", "+971501234567"}, {"US", "202-555-0123", "+12025550123"}, {"KR", "010-1234-5678", "+821012345678"}} {
		value, err := NormalizeRegionalPhone(tc.input, tc.country)
		if err != nil || value != tc.want {
			t.Fatalf("country %s: %s %v", tc.country, value, err)
		}
	}
	for _, value := range []string{"123", "+12025550123", "phone-text", "0501234567 ext. 123"} {
		if _, err := NormalizeRegionalPhone(value, "AE"); err == nil {
			t.Fatalf("invalid UAE input accepted: %q", value)
		}
	}
}
