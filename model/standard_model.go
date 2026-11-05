package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
	"starhub/setting"
)

type StandardModel struct {
	Model         string   `gorm:"primaryKey;size:128" json:"model"`
	Name          string   `gorm:"size:128;not null" json:"name"`
	Vendor        string   `gorm:"size:64;not null" json:"vendor"`
	Kind          string   `gorm:"size:32;not null" json:"kind"`
	Protocols     []string `gorm:"serializer:json;type:text" json:"protocols"`
	Capabilities  []string `gorm:"serializer:json;type:text" json:"capabilities"`
	ContextLength int64    `json:"context_length"`
	Description   string   `gorm:"type:text" json:"description"`
	Listed        bool     `gorm:"not null;default:false" json:"listed"`
	SortWeight    int64    `json:"sort_weight"`
	Revision      int64    `gorm:"not null;default:0" json:"revision"`
	CreatedAt     int64    `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     int64    `gorm:"autoUpdateTime" json:"updated_at"`
}
type StandardModelHistory struct {
	ID        int64  `gorm:"primaryKey" json:"id"`
	Model     string `gorm:"index;size:128" json:"model"`
	Before    string `gorm:"type:text" json:"before"`
	After     string `gorm:"type:text" json:"after"`
	ActorID   int64  `json:"actor_id"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
}
type StandardModelWrite struct {
	StandardModel
	Metadata                ModelMeta `json:"metadata"`
	ExpectedRevision        int64     `json:"expected_revision"`
	ExpectedCatalogRevision int64     `json:"expected_catalog_revision"`
	ExpectedPricingVersion  int64     `json:"expected_pricing_version"`
	PricingOptions          []Option  `json:"pricing_options"`
}
type ModelInventory struct {
	Corresponding int      `json:"corresponding"`
	Mapped        int      `json:"mapped"`
	Available     int64    `json:"available"`
	Groups        []string `json:"groups"`
}

var stableModelID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)

func ValidateOfficialReference(m *ModelMeta) error {
	for _, value := range []*float64{m.OfficialInputPerM, m.OfficialOutputPerM, m.OfficialCacheReadPerM, m.OfficialCacheWritePerM, m.OfficialPerRequest} {
		if value != nil && (*value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return ErrAdminInvalid
		}
	}
	present := m.OfficialInputPerM != nil || m.OfficialOutputPerM != nil || m.OfficialCacheReadPerM != nil || m.OfficialCacheWritePerM != nil || m.OfficialPerRequest != nil
	if !present && m.OfficialSourceURL == "" && m.OfficialVerifiedDate == "" {
		return nil
	}
	u, err := url.Parse(m.OfficialSourceURL)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || len(m.OfficialSourceURL) > 2048 {
		return fmt.Errorf("%w: official reference requires a source URL and verification date", ErrAdminInvalid)
	}
	if _, err := time.Parse("2006-01-02", m.OfficialVerifiedDate); err != nil {
		return fmt.Errorf("%w: official reference requires a source URL and verification date", ErrAdminInvalid)
	}
	for key := range u.Query() {
		if strings.Contains(strings.ToLower(key), "token") || strings.Contains(strings.ToLower(key), "secret") || strings.Contains(strings.ToLower(key), "password") || strings.EqualFold(key, "key") {
			return ErrAdminInvalid
		}
	}
	return nil
}
func StandardModelListed(ctx context.Context, name string) (bool, error) {
	var row StandardModel
	err := DB.WithContext(ctx).Select("model", "listed").First(&row, "model = ?", name).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	return row.Listed, err
}
func HiddenStandardModels(ctx context.Context) (map[string]bool, error) {
	var names []string
	err := DB.WithContext(ctx).Model(&StandardModel{}).Where("listed = ?", false).Pluck("model", &names).Error
	result := map[string]bool{}
	for _, name := range names {
		result[name] = true
	}
	return result, err
}
func modelInventory(tx *gorm.DB, name, vendor string) (ModelInventory, error) {
	out := ModelInventory{Groups: []string{}}
	var catalog ResourceCatalogRecord
	if err := tx.First(&catalog, 1).Error; err != nil {
		return out, err
	}
	var snapshot ResourceSnapshot
	if catalog.Snapshot != "" && json.Unmarshal([]byte(catalog.Snapshot), &snapshot) != nil {
		return out, ErrAdminInvalid
	}
	var placements []ResourcePlacementRecord
	if err := tx.Find(&placements).Error; err != nil {
		return out, err
	}
	mapped := map[string]string{}
	for _, p := range placements {
		if p.ModelID != nil {
			mapped[p.ResourceID] = *p.ModelID
		}
	}
	var profile DeploymentProfileVersion
	profileErr := tx.Order("revision DESC").First(&profile).Error
	if profileErr != nil && !errors.Is(profileErr, gorm.ErrRecordNotFound) {
		return out, profileErr
	}
	providers := map[string]string{}
	eligible := map[string]bool{}
	for _, c := range snapshot.Channels {
		providers[c.ID] = c.Provider
	}
	for _, r := range snapshot.Instances {
		if r.CanonicalModel != name && mapped[r.ID] != name {
			continue
		}
		if r.Status == "retired" {
			continue
		}
		if profile.Revision > 0 {
			if r.Region != profile.Profile.RegionID {
				continue
			}
			allowed := false
			for _, v := range profile.Profile.AllowedVendors {
				if strings.EqualFold(v, vendor) {
					allowed = true
				}
			}
			if !allowed {
				continue
			}
		}
		if provider := providers[r.ChannelID]; provider != "" && vendor != "" && !strings.EqualFold(provider, vendor) {
			continue
		}
		out.Corresponding++
		eligible[r.ID] = true
		if mapped[r.ID] == name {
			out.Mapped++
		}
	}
	for _, r := range snapshot.Instances {
		for _, p := range placements {
			if eligible[r.ID] && p.ResourceID == r.ID && p.ModelID != nil && *p.ModelID == name && p.Enabled && r.Status == "available" {
				var count int64
				if err := tx.Table("resource_gateway_managed_channels AS rgm").Joins("JOIN resource_gateway_routes AS rgr ON rgr.channel_id = rgm.channel_id").Joins("JOIN channels AS ch ON ch.id = rgm.channel_id").Where("rgm.active = 1 AND rgm.source_channel_id = ? AND rgr.model = ? AND ch.status = 1", r.ChannelID, name).Count(&count).Error; err != nil {
					return out, err
				}
				if count > 0 {
					out.Available++
				}
			}
		}
	}
	if err := tx.Model(&Ability{}).Where("model = ? AND enabled = ?", name, true).Distinct().Pluck("group", &out.Groups).Error; err != nil {
		return out, err
	}
	return out, nil
}
func ModelInventoryFor(ctx context.Context, name, vendor string) (ModelInventory, error) {
	return modelInventory(DB.WithContext(ctx), name, vendor)
}
func standardModelReferences(tx *gorm.DB, name string) error {
	checks := []struct {
		row    any
		clause string
		args   []any
	}{
		{&ResourcePlacementRecord{}, "model_id = ?", []any{name}},
		{&ResourceGatewayRoute{}, "model = ?", []any{name}},
		{&Token{}, "instr(',' || model_limits || ',', ',' || ? || ',') > 0", []any{name}},
		{&Log{}, "model = ? OR requested_model = ?", []any{name, name}},
		{&Task{}, "model = ?", []any{name}},
		{&Channel{}, "instr(',' || models || ',', ',' || ? || ',') > 0", []any{name}},
	}
	for _, c := range checks {
		var n int64
		if err := tx.Model(c.row).Where(c.clause, c.args...).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return ErrModelReferenced
		}
	}
	var catalog ResourceCatalogRecord
	if err := tx.First(&catalog, 1).Error; err != nil {
		return err
	}
	var snapshot ResourceSnapshot
	if catalog.Snapshot != "" {
		if err := json.Unmarshal([]byte(catalog.Snapshot), &snapshot); err != nil {
			return err
		}
		for _, r := range snapshot.Instances {
			if r.CanonicalModel == name {
				return ErrModelReferenced
			}
		}
	}
	return nil
}
func SaveStandardModel(ctx context.Context, w StandardModelWrite, actor int64, ip string) (StandardModel, error) {
	row := w.StandardModel
	if row.Capabilities == nil {
		row.Capabilities = []string{}
	}
	if !stableModelID.MatchString(row.Model) || row.Model == "myapis-auto" || row.Model == "veyra-auto" || !resourceText(row.Name, 128, true) || !resourceText(row.Vendor, 64, true) || !slices.Contains([]string{"chat", "embedding", "image", "audio", "video", "rerank"}, row.Kind) || row.ContextLength < 0 || row.ContextLength > maxModelMetadataRevision || len(row.Description) > 8192 || len(row.Protocols) == 0 || len(row.Protocols) > 16 || len(row.Capabilities) > 32 || w.ExpectedRevision < 0 {
		return row, ErrAdminInvalid
	}
	for _, p := range row.Protocols {
		if !slices.Contains([]string{"OpenAI", "OpenAIResponses", "Anthropic", "Gemini", "OpenAIEmbeddings", "OpenAIImages", "OpenAIAudio", "OpenAIVideo", "OpenAIRerank"}, p) {
			return row, ErrAdminInvalid
		}
	}
	for _, p := range append(append([]string{}, row.Protocols...), row.Capabilities...) {
		if !resourceText(p, 128, true) {
			return row, ErrAdminInvalid
		}
	}
	w.Metadata.Model = row.Model
	if err := ValidateOfficialReference(&w.Metadata); err != nil {
		return row, err
	}
	normalized := []Option{}
	seen := map[string]bool{}
	for _, o := range w.PricingOptions {
		if !isPricingOption(o.Key) || seen[o.Key] || len(o.Value) > maxPricingOptionBytes {
			return row, ErrAdminInvalid
		}
		seen[o.Key] = true
		v, err := setting.NormalizeOption(o.Key, o.Value)
		if err != nil {
			return row, err
		}
		normalized = append(normalized, Option{Key: o.Key, Value: v})
	}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&ModelMetadataState{}).Where("id = 1").UpdateColumn("revision", gorm.Expr("revision")).Error; err != nil {
			return err
		}
		var state ModelMetadataState
		if err := tx.First(&state, 1).Error; err != nil {
			return err
		}
		if state.Revision != w.ExpectedCatalogRevision {
			return ErrAdminConflict
		}
		var old StandardModel
		err := tx.First(&old, "model = ?", row.Model).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if old.Revision != w.ExpectedRevision {
			return ErrAdminConflict
		}
		if old.Revision == 0 {
			configured := false
			for _, option := range normalized {
				if option.Key != "model_ratio" && option.Key != "model_price" && option.Key != "model_price_expr" {
					continue
				}
				var values map[string]json.RawMessage
				if json.Unmarshal([]byte(option.Value), &values) == nil && len(values[row.Model]) > 0 {
					configured = true
				}
			}
			for _, key := range []string{"model_ratio", "model_price", "model_price_expr", "model_time_token_prices"} {
				var option Option
				if err := tx.First(&option, "`key` = ?", key).Error; err == nil {
					var values map[string]json.RawMessage
					if json.Unmarshal([]byte(option.Value), &values) == nil && len(values[row.Model]) > 0 {
						configured = true
					}
				} else if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
			}
			if !configured {
				return fmt.Errorf("%w: initial platform pricing is required", ErrAdminInvalid)
			}
		}
		if row.Listed {
			inventory, err := modelInventory(tx, row.Model, row.Vendor)
			if err != nil {
				return err
			}
			if inventory.Corresponding == 0 {
				return fmt.Errorf("%w: no corresponding resource in the permitted region/vendor scope", ErrAdminInvalid)
			}
		}
		if len(normalized) > 0 {
			if err := tx.Model(&OptionRevision{}).Where("id = 1").UpdateColumn("version", gorm.Expr("version")).Error; err != nil {
				return err
			}
			var rev OptionRevision
			if err := tx.First(&rev, 1).Error; err != nil {
				return err
			}
			if rev.Version != w.ExpectedPricingVersion {
				return ErrAdminConflict
			}
			for _, o := range normalized {
				if _, err := saveOptionTx(tx, o.Key, o.Value, actor, -1); err != nil {
					return err
				}
			}
		}
		row.Revision = old.Revision + 1
		row.CreatedAt = old.CreatedAt
		row.UpdatedAt = 0
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		var oldMeta ModelMeta
		if err := tx.First(&oldMeta, "model = ?", row.Model).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		meta := w.Metadata
		meta.CreatedAt = oldMeta.CreatedAt
		meta.Vendor = row.Vendor
		meta.Description = row.Description
		meta.ContextLength = row.ContextLength
		meta.Tags = strings.Join(row.Capabilities, ",")
		meta.SortWeight = row.SortWeight
		if err := tx.Save(&meta).Error; err != nil {
			return err
		}
		if err := tx.Model(&state).Update("revision", state.Revision+1).Error; err != nil {
			return err
		}
		beforeState := map[string]any{"model": old, "reference": oldMeta}
		afterState := map[string]any{"model": row, "reference": meta}
		before, _ := json.Marshal(beforeState)
		after, _ := json.Marshal(afterState)
		if err := tx.Create(&StandardModelHistory{Model: row.Model, Before: string(before), After: string(after), ActorID: actor}).Error; err != nil {
			return err
		}
		return RecordAdminChange(tx, actor, "standard_model_saved", "models", row.Model, beforeState, afterState, "", ip)
	})
	return row, err
}
func DeleteStandardModel(ctx context.Context, name string, expected, actor int64, ip string) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&ModelMetadataState{}).Where("id = 1").UpdateColumn("revision", gorm.Expr("revision")).Error; err != nil {
			return err
		}
		var state ModelMetadataState
		if err := tx.First(&state, 1).Error; err != nil {
			return err
		}
		if state.Revision != expected {
			return ErrAdminConflict
		}
		var row StandardModel
		if err := tx.First(&row, "model = ?", name).Error; err != nil {
			return err
		}
		if err := standardModelReferences(tx, name); err != nil {
			return err
		}
		if err := tx.Delete(&row).Error; err != nil {
			return err
		}
		if err := tx.Where("model = ?", name).Delete(&ModelMeta{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&state).Update("revision", state.Revision+1).Error; err != nil {
			return err
		}
		return RecordAdminChange(tx, actor, "standard_model_deleted", "models", name, row, nil, "", ip)
	})
}
