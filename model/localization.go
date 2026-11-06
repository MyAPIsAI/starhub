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
	policy := LocalizationPolicy{DefaultLanguage: profile.Profile.DefaultLanguage, Languages: []string{}}
	for _, lang := range profile.Profile.Languages {
		if lang == "ar" && !profile.Profile.Features["arabic"] {
			continue
		}
		if slices.Contains([]string{"en", "zh", "ar"}, lang) {
			policy.Languages = append(policy.Languages, lang)
		}
	}
	if len(policy.Languages) == 0 {
		policy.Languages = []string{"en"}
	}
	if !slices.Contains(policy.Languages, policy.DefaultLanguage) {
		policy.DefaultLanguage = policy.Languages[0]
		if slices.Contains(policy.Languages, "en") {
			policy.DefaultLanguage = "en"
		}
	}
	return policy, nil
}

func EffectiveAccountLanguage(ctx context.Context, language string) (string, error) {
	policy, err := ReadLocalizationPolicy(ctx)
	if err != nil {
		return "", err
	}
	if slices.Contains(policy.Languages, language) {
		return language, nil
	}
	return policy.DefaultLanguage, nil
}
