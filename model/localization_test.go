package model

import (
	"slices"
	"testing"
)

func TestArabicFallbackRemainsSelectableWhenProfileOmitsEnglish(t *testing.T) {
	profile := DefaultDeploymentProfile()
	profile.DefaultLanguage = "zh"
	profile.Languages = []string{"zh", "ar"}
	profile.Features["arabic"] = false
	policy := localizationPolicyForProfile(profile)
	if slices.Contains(policy.Languages, "ar") || !slices.Contains(policy.Languages, "en") || effectivePolicyLanguage(policy, "ar") != "en" {
		t.Fatalf("English fallback is not selectable: %+v", policy)
	}
	if policy.DefaultLanguage != "zh" || effectivePolicyLanguage(policy, "") != "zh" || effectivePolicyLanguage(policy, "zh") != "zh" {
		t.Fatal("Arabic fallback changed unrelated language preferences")
	}
}
