package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"starhub/common"
	"starhub/config"
)

var ErrAdminConflict = errors.New("administration state changed; reload and review")
var ErrAdminInvalid = errors.New("invalid administration configuration")
var ErrModelReferenced = errors.New("model has resource mappings, Key authorizations or historical references")

// Only explicitly constructed, credential-free summaries enter this API.
func RecordAdminChange(tx *gorm.DB, actor int64, action, kind, id string, before, after any, reason, ip string) error {
	var user User
	if err := tx.Select("id", "username", "role").First(&user, actor).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		user.ID = actor
		user.Username = "unknown"
	}
	if ip == "" {
		ip, _ = tx.Statement.Context.Value(common.CtxAdminIP).(string)
	}
	encode := func(v any) string {
		if v == nil {
			return ""
		}
		b, _ := json.Marshal(v)
		return string(b)
	}
	target, _ := strconv.ParseInt(id, 10, 64)
	return tx.Create(&AuditLog{UserID: actor, Username: user.Username, ActorRole: user.Role, Action: action, ObjectType: kind, ObjectID: id, TargetID: target, BeforeSummary: encode(before), AfterSummary: encode(after), Reason: reason, IP: ip, Method: "CHANGE", Path: "/api/admin/" + kind, Status: 200}).Error
}

type DeploymentProfile struct {
	RegionID            string            `json:"region_id"`
	Brand               string            `json:"brand"`
	Domain              string            `json:"domain"`
	LogoURL             string            `json:"logo_url"`
	DefaultLanguage     string            `json:"default_language"`
	Languages           []string          `json:"languages"`
	Timezone            string            `json:"timezone"`
	Currency            string            `json:"currency"`
	AllowedVendors      []string          `json:"allowed_vendors"`
	LoginMethods        []string          `json:"login_methods"`
	PaymentChannels     []string          `json:"payment_channels"`
	PaymentConfigRefs   map[string]string `json:"payment_config_refs"`
	CookieConsent       bool              `json:"cookie_consent"`
	Features            map[string]bool   `json:"features"`
	StoreRequestContent bool              `json:"store_request_content"`
}
type DeploymentProfileState struct {
	ID       int   `gorm:"primaryKey"`
	Revision int64 `gorm:"not null;default:0"`
}
type DeploymentProfileVersion struct {
	Revision  int64             `gorm:"primaryKey;autoIncrement:false" json:"revision"`
	Profile   DeploymentProfile `gorm:"serializer:json;type:text;not null" json:"profile"`
	ActorID   int64             `json:"actor_id"`
	CreatedAt int64             `gorm:"autoCreateTime" json:"created_at"`
}

func DefaultDeploymentProfile() DeploymentProfile {
	return DeploymentProfile{RegionID: "default", Brand: "MyAPIs", DefaultLanguage: "en", Languages: []string{"en", "zh", "ar"}, Timezone: config.RegionTimezone(), Currency: "USD", AllowedVendors: []string{}, LoginMethods: []string{"password"}, PaymentChannels: []string{}, PaymentConfigRefs: map[string]string{}, Features: map[string]bool{"playground": true, "models": true, "status": true, "smart_entry": true, "video": true, "arabic": true}}
}
func CurrentDeploymentProfile(ctx context.Context) (DeploymentProfileVersion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var row DeploymentProfileVersion
	err := DB.WithContext(ctx).Order("revision DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row.Profile = DefaultDeploymentProfile()
		return row, nil
	}
	return row, err
}
func ValidateDeploymentProfile(p DeploymentProfile) error {
	if !resourceText(p.RegionID, 64, true) || !resourceText(p.Brand, 128, true) || p.Currency != "USD" || p.StoreRequestContent {
		return fmt.Errorf("%w: identity, USD currency or content retention", ErrAdminInvalid)
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return fmt.Errorf("%w: timezone", ErrAdminInvalid)
	}
	if !slices.Contains(p.Languages, p.DefaultLanguage) || len(p.Languages) > 3 || len(p.Languages) == 0 {
		return fmt.Errorf("%w: languages", ErrAdminInvalid)
	}
	seen := map[string]bool{}
	for _, lang := range p.Languages {
		if !slices.Contains([]string{"en", "zh", "ar"}, lang) || seen[lang] {
			return fmt.Errorf("%w: languages", ErrAdminInvalid)
		}
		seen[lang] = true
	}
	if len(p.AllowedVendors) > 100 || len(p.Features) > 20 || len(p.LoginMethods) > 8 || len(p.PaymentChannels) > 20 || len(p.PaymentConfigRefs) > 20 {
		return ErrAdminInvalid
	}
	for _, value := range append(append(append([]string{}, p.AllowedVendors...), p.LoginMethods...), p.PaymentChannels...) {
		if !resourceText(value, 128, true) {
			return ErrAdminInvalid
		}
	}
	if p.Domain != "" {
		u, err := url.Parse("https://" + p.Domain)
		if err != nil || u.Host != p.Domain || u.User != nil || u.Path != "" {
			return fmt.Errorf("%w: domain", ErrAdminInvalid)
		}
	}
	if p.LogoURL != "" {
		u, err := url.Parse(p.LogoURL)
		if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return fmt.Errorf("%w: logo URL", ErrAdminInvalid)
		}
	}
	for k, v := range p.PaymentConfigRefs {
		if !resourceText(k, 64, true) || !resourceText(v, 128, true) || !strings.HasPrefix(v, "config:") {
			return fmt.Errorf("%w: payment settings require deployment configuration references", ErrAdminInvalid)
		}
	}
	for k := range p.Features {
		if !slices.Contains([]string{"playground", "models", "status", "smart_entry", "video", "arabic"}, k) {
			return ErrAdminInvalid
		}
	}
	return nil
}
func SaveDeploymentProfile(ctx context.Context, p DeploymentProfile, expected, actor int64, ip string) (DeploymentProfileVersion, error) {
	result := DeploymentProfileVersion{}
	if err := ValidateDeploymentProfile(p); err != nil {
		return result, err
	}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&DeploymentProfileState{}).Where("id = 1").UpdateColumn("revision", gorm.Expr("revision")).Error; err != nil {
			return err
		}
		var state DeploymentProfileState
		if err := tx.First(&state, 1).Error; err != nil {
			return err
		}
		if expected != state.Revision {
			return ErrAdminConflict
		}
		var old DeploymentProfileVersion
		err := tx.Last(&old, "revision = ?", expected).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		result = DeploymentProfileVersion{Revision: expected + 1, Profile: p, ActorID: actor}
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		if err := tx.Model(&state).Update("revision", result.Revision).Error; err != nil {
			return err
		}
		return RecordAdminChange(tx, actor, "deployment_profile_updated", "region-profile", p.RegionID, old.Profile, p, "", ip)
	})
	return result, err
}

type LegalDocumentVersion struct {
	ID          int64  `gorm:"primaryKey" json:"id"`
	Kind        string `gorm:"size:32;uniqueIndex:idx_legal_version,priority:1" json:"kind"`
	Language    string `gorm:"size:8;uniqueIndex:idx_legal_version,priority:2" json:"language"`
	Version     string `gorm:"size:64;uniqueIndex:idx_legal_version,priority:3" json:"version"`
	Content     string `gorm:"type:text;not null" json:"content"`
	EffectiveAt int64  `gorm:"index;not null" json:"effective_at"`
	Timezone    string `gorm:"size:64;not null" json:"timezone"`
	ActorID     int64  `json:"actor_id"`
	CreatedAt   int64  `gorm:"autoCreateTime" json:"created_at"`
}

func SaveLegalDocument(ctx context.Context, d LegalDocumentVersion, localTime string, actor int64, ip string) (LegalDocumentVersion, error) {
	if !slices.Contains([]string{"terms", "privacy", "cookies", "refund"}, d.Kind) || !slices.Contains([]string{"en", "zh", "ar"}, d.Language) || !resourceText(d.Version, 64, true) || strings.TrimSpace(d.Content) == "" || len(d.Content) > 1<<20 {
		return d, ErrAdminInvalid
	}
	profile, err := CurrentDeploymentProfile(ctx)
	if err != nil {
		return d, err
	}
	loc, err := time.LoadLocation(profile.Profile.Timezone)
	if err != nil {
		return d, err
	}
	at, err := time.ParseInLocation("2006-01-02T15:04", localTime, loc)
	if err != nil || at.Format("2006-01-02T15:04") != localTime {
		return d, fmt.Errorf("%w: effective local time", ErrAdminInvalid)
	}
	d.ID = 0
	d.EffectiveAt = at.Unix()
	d.Timezone = loc.String()
	d.ActorID = actor
	d.CreatedAt = 0
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&LegalDocumentVersion{}).Where("kind = ? AND language = ? AND (version = ? OR effective_at = ?)", d.Kind, d.Language, d.Version, d.EffectiveAt).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrAdminConflict
		}
		if err := tx.Create(&d).Error; err != nil {
			return err
		}
		return RecordAdminChange(tx, actor, "legal_version_scheduled", "legal", d.Kind+"/"+d.Language, nil, map[string]any{"version": d.Version, "effective_at": d.EffectiveAt, "timezone": d.Timezone}, "", ip)
	})
	return d, err
}
func ActiveLegalDocument(ctx context.Context, kind, language string, at time.Time) (LegalDocumentVersion, error) {
	var d LegalDocumentVersion
	err := DB.WithContext(ctx).Where("kind = ? AND language = ? AND effective_at <= ?", kind, language, at.Unix()).Order("effective_at DESC, id DESC").First(&d).Error
	return d, err
}

// Retention never inspects request bodies, credentials or financial ledgers.
func PruneAdministrationAudit(ctx context.Context, now time.Time) error {
	return DB.WithContext(ctx).Where("created_at < ?", now.AddDate(-1, 0, 0).Unix()).Delete(&AuditLog{}).Error
}
