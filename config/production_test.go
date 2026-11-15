package config

import "testing"

func TestProductionRejectsDisposableConfiguration(t *testing.T) {
	for _, tc := range []struct {
		mode, secret, password string
		valid                  bool
	}{
		{"", "", "", true},
		{"production", "", "LongInitialPassword93", false},
		{"production", "persistent-secret-with-32-characters", "123456", false},
		{"production", "persistent-secret-with-32-characters", "LongInitialPassword93", true},
		{"prodution", "", "", false},
	} {
		env := startupEnv(map[string]string{"STARHUB_ENV": tc.mode, "STARHUB_CRYPTO_SECRET": tc.secret})
		if err := ValidateProduction(Bootstrap{RootPassword: tc.password}, env); (err == nil) != tc.valid {
			t.Fatalf("mode %q: unexpected validation %v", tc.mode, err)
		}
	}
}
