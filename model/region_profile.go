package model

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrRegionProfileNotFound = errors.New("region profile not found")

// RegionProfile is the commercial region configuration that drives
// language defaults, currency display, timezone, legal version and feature
// flags. It is distinct from ResourceRegion (which is an inventory label).
type RegionProfile struct {
	ID                 string `gorm:"primaryKey;size:64" json:"id"`
	Name               string `gorm:"size:128;not null" json:"name"`
	DefaultLanguage    string `gorm:"size:8;not null;default:'en'" json:"default_language"`
	SupportedLanguages string `gorm:"size:128;not null;default:'en'" json:"supported_languages"` // comma-separated
	Currency           string `gorm:"size:8;not null;default:'USD'" json:"currency"`
	Timezone           string `gorm:"size:64;not null;default:'UTC'" json:"timezone"`
	CountryCode        string `gorm:"size:8;not null;default:''" json:"country_code"`
	AllowedVendors     string `gorm:"size:512;not null;default:''" json:"allowed_vendors"` // comma-separated; empty = all
	AllowedModels      string `gorm:"size:1024;not null;default:''" json:"allowed_models"` // comma-separated; empty = all
	AuthPlugins        string `gorm:"size:256;not null;default:''" json:"auth_plugins"`
	PaymentPlugins     string `gorm:"size:256;not null;default:''" json:"payment_plugins"`
	LegalVersion       string `gorm:"size:32;not null;default:''" json:"legal_version"`
	FeatureFlags       string `gorm:"type:text;not null;default:'{}'" json:"feature_flags"` // JSON
	Enabled            bool   `gorm:"not null;default:true" json:"enabled"`
	Revision           int64  `gorm:"not null;default:0" json:"revision"`
	CreatedAt          int64  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt          int64  `gorm:"autoUpdateTime" json:"updated_at"`
}

// SupportedLanguageList returns the supported languages as a slice.
func (r *RegionProfile) SupportedLanguageList() []string {
	return SplitList(r.SupportedLanguages)
}

// IsVendorAllowed reports whether the vendor is in the allowed list.
// An empty allowed list means all vendors are permitted.
func (r *RegionProfile) IsVendorAllowed(vendor string) bool {
	if r.AllowedVendors == "" {
		return true
	}
	for _, v := range SplitList(r.AllowedVendors) {
		if v == vendor {
			return true
		}
	}
	return false
}

// IsModelAllowed reports whether the model is in the allowed list.
// An empty allowed list means all models are permitted.
func (r *RegionProfile) IsModelAllowed(model string) bool {
	if r.AllowedModels == "" {
		return true
	}
	for _, m := range SplitList(r.AllowedModels) {
		if m == model {
			return true
		}
	}
	return false
}

// SaveRegionProfile creates or updates a region profile with optimistic
// concurrency control via the Revision field.
func SaveRegionProfile(profile *RegionProfile, expectedRevision int64) error {
	profile.ID = strings.TrimSpace(profile.ID)
	if profile.ID == "" || len(profile.ID) > 64 {
		return errors.New("region profile id must be 1-64 characters")
	}
	if profile.Name == "" {
		return errors.New("region profile name is required")
	}
	if profile.Timezone == "" {
		profile.Timezone = "UTC"
	}
	if profile.DefaultLanguage == "" {
		profile.DefaultLanguage = "en"
	}
	if profile.SupportedLanguages == "" {
		profile.SupportedLanguages = profile.DefaultLanguage
	}
	if profile.Currency == "" {
		profile.Currency = "USD"
	}
	if profile.FeatureFlags == "" {
		profile.FeatureFlags = "{}"
	}
	var flags map[string]bool
	if json.Unmarshal([]byte(profile.FeatureFlags), &flags) != nil || flags == nil {
		return errors.New("feature flags must be a JSON object of boolean values")
	}
	foundDefault := false
	for _, lang := range profile.SupportedLanguageList() {
		if lang != "zh" && lang != "en" && lang != "ar" && lang != "ko" {
			return errors.New("unsupported region language")
		}
		foundDefault = foundDefault || lang == profile.DefaultLanguage
	}
	if !foundDefault {
		return errors.New("default language must be supported by the region")
	}
	// Validate timezone.
	if profile.Timezone != "" {
		if _, err := time.LoadLocation(profile.Timezone); err != nil {
			return errors.New("invalid timezone: " + profile.Timezone)
		}
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var existing RegionProfile
		err := tx.Where("id = ?", profile.ID).First(&existing).Error
		if err == nil {
			// Update: check revision.
			if expectedRevision >= 0 && existing.Revision != expectedRevision {
				return errors.New("region profile was modified by another admin; reload and retry")
			}
			profile.Revision = existing.Revision + 1
			profile.CreatedAt = existing.CreatedAt
			return tx.Save(profile).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// Create.
		profile.Revision = 0
		return tx.Create(profile).Error
	})
}

// GetRegionProfile returns a region profile by ID.
func GetRegionProfile(id string) (*RegionProfile, error) {
	var profile RegionProfile
	err := DB.Where("id = ?", id).First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRegionProfileNotFound
	}
	return &profile, err
}

// ListRegionProfiles returns all region profiles.
func ListRegionProfiles() ([]RegionProfile, error) {
	var profiles []RegionProfile
	err := DB.Order("id ASC").Find(&profiles).Error
	return profiles, err
}

// DeleteRegionProfile removes a region profile. Refuses if it is the only
// enabled profile (must always have at least one).
func DeleteRegionProfile(id string) error {
	if id == ActiveRegionID() {
		return errors.New("cannot delete the active region profile")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&RegionProfile{}).Where("enabled = true").Count(&count).Error; err != nil {
			return err
		}
		if count <= 1 {
			var profile RegionProfile
			if err := tx.Where("id = ? AND enabled = true", id).First(&profile).Error; err == nil {
				return errors.New("cannot delete the last enabled region profile")
			}
		}
		res := tx.Where("id = ?", id).Delete(&RegionProfile{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrRegionProfileNotFound
		}
		return nil
	})
}
