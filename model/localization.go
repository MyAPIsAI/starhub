package model

import (
	"context"
	"slices"
)

type LocalizationPolicy struct {
	DefaultLanguage string   `json:"default_language"`
	Languages       []string `json:"languages"`
}

func ReadLocalizationPolicy(ctx context.Context) (LocalizationPolicy, error) {
	profile, err := CurrentDeploymentProfile(ctx)
	if err != nil {
		return LocalizationPolicy{}, err
	}
	return localizationPolicyForProfile(profile.Profile), nil
}

func localizationPolicyForProfile(profile DeploymentProfile) LocalizationPolicy {
	policy := LocalizationPolicy{DefaultLanguage: profile.DefaultLanguage, Languages: []string{}}
	for _, lang := range profile.Languages {
		if lang == "ar" && !profile.Features["arabic"] {
			continue
		}
		if slices.Contains([]string{"en", "zh", "ar"}, lang) {
			policy.Languages = append(policy.Languages, lang)
		}
	}
	// English remains available for accounts whose Arabic preference is disabled.
	if !slices.Contains(policy.Languages, "ar") && !slices.Contains(policy.Languages, "en") {
		policy.Languages = append([]string{"en"}, policy.Languages...)
	}
	if !slices.Contains(policy.Languages, policy.DefaultLanguage) {
		policy.DefaultLanguage = policy.Languages[0]
		if slices.Contains(policy.Languages, "en") {
			policy.DefaultLanguage = "en"
		}
	}
	return policy
}

func effectivePolicyLanguage(policy LocalizationPolicy, language string) string {
	if slices.Contains(policy.Languages, language) {
		return language
	}
	if language == "ar" {
		return "en"
	}
	return policy.DefaultLanguage
}

func EffectiveAccountLanguage(ctx context.Context, language string) (string, error) {
	policy, err := ReadLocalizationPolicy(ctx)
	if err != nil {
		return "", err
	}
	return effectivePolicyLanguage(policy, language), nil
}
