package model

import (
	"context"
	"encoding/json"
	"log"
	"slices"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"
	"starhub/common"
	"starhub/setting"
)

const PublicCatalogGrace = 10 * time.Minute

// CatalogAvailability is display history, never a source of routing permission.
type CatalogAvailability struct {
	Model            string `gorm:"primaryKey;size:128"`
	GroupID          string `gorm:"primaryKey;size:64"`
	DefinitionID     int64
	EverAvailable    bool
	Available        bool
	UnavailableSince int64
	ObservedAt       int64
	ChannelTypes     string `gorm:"type:text"`
}

func (s CatalogAvailability) Visible(at time.Time) bool {
	return s.Available || (s.EverAvailable && s.UnavailableSince > 0 && at.UnixMilli()-s.UnavailableSince < PublicCatalogGrace.Milliseconds())
}

type PublicCatalogGroup struct {
	ServiceGroupConfig
	Ratio float64
}

type PublicCatalogState struct {
	Names        []string
	Groups       []PublicCatalogGroup
	Metadata     map[string]ModelMeta
	Standards    map[string]StandardModel
	Availability map[string]map[string]CatalogAvailability
}

var publicCatalogMu sync.Mutex

// ReadPublicCatalogState observes published resources independently of visitors.
// A transaction orders competing pollers and preserves the outage start on restart.
func ReadPublicCatalogState(ctx context.Context, at time.Time) (*PublicCatalogState, error) {
	publicCatalogMu.Lock()
	defer publicCatalogMu.Unlock()
	out := &PublicCatalogState{Groups: []PublicCatalogGroup{}, Metadata: map[string]ModelMeta{}, Standards: map[string]StandardModel{}, Availability: map[string]map[string]CatalogAvailability{}}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var option Option
		if err := tx.Where("key = ?", setting.OptionKeyGroupRatio).First(&option).Error; err != nil {
			return err
		}
		var ratios map[string]float64
		if err := json.Unmarshal([]byte(option.Value), &ratios); err != nil {
			return err
		}
		var groups []ServiceGroupConfig
		if err := tx.Order("group_id").Find(&groups).Error; err != nil {
			return err
		}
		var deleted []ServiceGroupDeletion
		if err := tx.Find(&deleted).Error; err != nil {
			return err
		}
		excluded := map[string]bool{}
		for _, g := range deleted {
			excluded[g.GroupID] = true
		}
		for _, g := range groups {
			excluded[g.GroupID] = true
			if ratio, ok := ratios[g.GroupID]; ok && g.Enabled && g.Visibility == "common" {
				out.Groups = append(out.Groups, PublicCatalogGroup{ServiceGroupConfig: g, Ratio: ratio})
			}
		}
		// Explicit legacy ratio entries retain their public meaning; no default is invented.
		for id, ratio := range ratios {
			if !excluded[id] {
				out.Groups = append(out.Groups, PublicCatalogGroup{ServiceGroupConfig: ServiceGroupConfig{GroupID: id, Name: id, Enabled: true, Visibility: "common"}, Ratio: ratio})
			}
		}
		sort.Slice(out.Groups, func(i, j int) bool {
			if out.Groups[i].Ratio != out.Groups[j].Ratio {
				return out.Groups[i].Ratio < out.Groups[j].Ratio
			}
			return out.Groups[i].GroupID < out.Groups[j].GroupID
		})
		var metadata []ModelMeta
		if err := tx.Find(&metadata).Error; err != nil {
			return err
		}
		names := map[string]bool{}
		for _, m := range metadata {
			out.Metadata[m.Model] = m
			names[m.Model] = true
		}
		var standards []StandardModel
		if err := tx.Find(&standards).Error; err != nil {
			return err
		}
		for _, m := range standards {
			out.Standards[m.Model] = m
			names[m.Model] = true
		}
		var old []CatalogAvailability
		if err := tx.Find(&old).Error; err != nil {
			return err
		}
		states := map[string]map[string]CatalogAvailability{}
		for _, s := range old {
			if states[s.Model] == nil {
				states[s.Model] = map[string]CatalogAvailability{}
			}
			states[s.Model][s.GroupID] = s
			names[s.Model] = true
		}
		type covered struct {
			Model           string
			GroupID         string
			ChannelType     int
			SourceChannelID string
		}
		var coverage []covered
		if err := tx.Table("abilities AS a").Select("a.model, a.\"group\" AS group_id, c.type AS channel_type, COALESCE(rgm.source_channel_id, '') AS source_channel_id").
			Joins("JOIN channels c ON c.id = a.channel_id").
			Joins("LEFT JOIN resource_gateway_managed_channels rgm ON rgm.channel_id = c.id").
			Where("a.enabled = ? AND c.status = ?", true, common.ChannelStatusEnabled).
			Where(resourceGatewayAbilityCondition("a")).Scan(&coverage).Error; err != nil {
			return err
		}
		// Fresh reports can establish an all-gateway open circuit. Unknown reports
		// do not manufacture an outage or expose gateway identities to visitors.
		catalog, err := readResourceCatalog(tx)
		if err != nil {
			return err
		}
		identities := map[string]string{}
		if catalog.Snapshot != nil {
			for _, r := range catalog.Snapshot.Instances {
				identities[r.ID] = resourceRuntimeIdentity(r)
			}
		}
		var reports []GatewayResourceReport
		if err := tx.Where("at >= ? AND at <= ?", at.Add(-15*time.Second).Unix(), at.Unix()).Find(&reports).Error; err != nil {
			return err
		}
		seen, healthy := map[string]bool{}, map[string]bool{}
		for _, report := range reports {
			var resources []ResourceLocalState
			if err := json.Unmarshal([]byte(report.Resources), &resources); err != nil {
				return err
			}
			for _, r := range resources {
				if identities[r.ResourceID] != r.Identity {
					continue
				}
				seen[r.ResourceID] = true
				if r.State != "open" {
					healthy[r.ResourceID] = true
				}
			}
		}
		actual := map[string]map[string][]int{}
		for _, c := range coverage {
			names[c.Model] = true
			if c.SourceChannelID != "" {
				usable := false
				if catalog.Snapshot != nil {
					for _, r := range catalog.Snapshot.Instances {
						p := catalog.Placements[r.ID]
						if r.ChannelID == c.SourceChannelID && r.Status == "available" && p.Enabled && p.ModelID != nil && *p.ModelID == c.Model && slices.Contains(p.GroupIDs, c.GroupID) && (!seen[r.ID] || healthy[r.ID]) {
							usable = true
							break
						}
					}
				}
				if !usable {
					continue
				}
			}
			if actual[c.Model] == nil {
				actual[c.Model] = map[string][]int{}
			}
			if !slices.Contains(actual[c.Model][c.GroupID], c.ChannelType) {
				actual[c.Model][c.GroupID] = append(actual[c.Model][c.GroupID], c.ChannelType)
			}
		}
		for name := range names {
			if name == "myapis-auto" || name == "veyra-auto" {
				continue
			}
			if m, ok := out.Standards[name]; ok && !m.Listed {
				if len(states[name]) > 0 {
					if err := tx.Where("model = ?", name).Delete(&CatalogAvailability{}).Error; err != nil {
						return err
					}
				}
				continue
			}
			out.Availability[name] = map[string]CatalogAvailability{}
			_, published := out.Standards[name]
			for _, g := range out.Groups {
				s := states[name][g.GroupID]
				previous := s
				if s.Model == "" || s.DefinitionID != g.ID {
					s = CatalogAvailability{Model: name, GroupID: g.GroupID, DefinitionID: g.ID}
				}
				if at.UnixMilli() >= s.ObservedAt {
					available := len(actual[name][g.GroupID]) > 0
					if available {
						s.EverAvailable = true
						s.UnavailableSince = 0
						b, _ := json.Marshal(actual[name][g.GroupID])
						s.ChannelTypes = string(b)
					} else if s.Available || s.UnavailableSince == 0 {
						s.UnavailableSince = at.UnixMilli()
					}
					s.Available = available
					s.ObservedAt = at.UnixMilli()
					// Persist transitions, not a write for every anonymous page read.
					previous.ObservedAt = s.ObservedAt
					if previous != s {
						if err := tx.Save(&s).Error; err != nil {
							return err
						}
					}
				}
				out.Availability[name][g.GroupID] = s
				published = published || s.EverAvailable
			}
			// Legacy channels become public only after serving a common group.
			// Private-only upstream model names are not a published model registry.
			if published {
				out.Names = append(out.Names, name)
			}
		}
		sort.Strings(out.Names)
		return nil
	})
	return out, err
}

func WatchPublicCatalog(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if _, err := ReadPublicCatalogState(ctx, time.Now()); err != nil {
			log.Printf("public catalog observation failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
