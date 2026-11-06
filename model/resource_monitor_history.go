package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"starhub/config"
)

type ResourceSnapshotStatus struct {
	Sequence    int64  `json:"sequence"`
	PublishedAt *int64 `json:"published_at"`
	AgeSeconds  *int64 `json:"age_seconds"`
	Status      string `json:"status"`
}

func resourceSnapshotStatus(snapshot *ResourceSnapshot, now time.Time) ResourceSnapshotStatus {
	result := ResourceSnapshotStatus{Status: "missing"}
	if snapshot == nil {
		return result
	}
	result.Sequence, result.Status = snapshot.Sequence, "unknown"
	published, err := time.Parse(time.RFC3339Nano, snapshot.PublishedAt)
	if err != nil {
		return result
	}
	at, age := published.Unix(), now.Unix()-published.Unix()
	result.PublishedAt, result.AgeSeconds = &at, &age
	result.Status = "fresh"
	if age > 180 {
		result.Status = "expired"
	}
	if age < 0 {
		result.Status = "clock_skew"
	}
	return result
}

type ResourceMonitorFeed struct {
	ID             string `gorm:"primaryKey" json:"-"`
	Kind           string `json:"kind"`
	Version        int64  `json:"version"`
	Initialized    bool   `json:"initialized"`
	BaselineDigest string `json:"-"`
	LastAttemptAt  int64  `json:"last_attempt_at"`
	LastSuccessAt  int64  `json:"last_success_at"`
	ErrorCode      string `json:"error_code"`
	NextAttemptAt  int64  `json:"-"`
	Lease          string `json:"-"`
	LeaseUntil     int64  `json:"-"`
}

type ResourceDirectoryPrice struct {
	Source     string `gorm:"primaryKey" json:"-"`
	ResourceID string `gorm:"primaryKey" json:"resource_id"`
	PriceTier  int    `json:"price_tier"`
	Version    int64  `json:"version"`
}

type ResourceObservedEvent struct {
	ID      string `gorm:"primaryKey"`
	Source  string `gorm:"uniqueIndex:idx_resource_observed_version"`
	Version int64  `gorm:"uniqueIndex:idx_resource_observed_version"`
	Digest  string
}

type ResourcePriceMismatch struct {
	ID               string `gorm:"primaryKey" json:"id"`
	Source           string `gorm:"index" json:"-"`
	ResourceID       string `json:"resource_id"`
	SnapshotSequence int64  `json:"snapshot_sequence"`
	SnapshotPrice    int    `json:"snapshot_price"`
	EventPrice       int    `json:"event_price"`
	EventVersion     int64  `json:"event_version"`
	DetectedAt       int64  `gorm:"index" json:"at"`
	ResolvedAt       int64  `json:"resolved_at"`
}

type ResourcePriceEvent struct {
	EventID    string `json:"event_id"`
	Version    int64  `json:"version"`
	Type       string `json:"type"`
	OccurredAt string `json:"occurred_at"`
	Resource   struct {
		ResourceID string `json:"resource_id"`
		PriceTier  *int   `json:"price_tier,omitempty"`
	} `json:"resource"`
}

// This feed projects directory prices for monitoring; it never acknowledges credential application.
type ResourcePriceEventBatch struct {
	BaseVersion int64                     `json:"base_version"`
	Baseline    *[]ResourceDirectoryPrice `json:"baseline,omitempty"`
	Events      []ResourcePriceEvent      `json:"events"`
}

func monitorHash(parts ...any) string {
	raw, _ := json.Marshal(parts)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func resourceMonitorSourceID() string {
	s := config.ResourceMonitoringSource()
	return monitorHash(config.ResourceSnapshotSource().URL, s.EventsURL, s.SnapshotURL)
}

func monitorFeedID(kind string) string { return resourceMonitorSourceID() + ":" + kind }
func validPrice(p int) bool            { return p == 4 || p == 6 || p == 8 || p == 10 }

func applyResourcePriceEvents(tx *gorm.DB, batch ResourcePriceEventBatch, now time.Time) error {
	if batch.BaseVersion < 0 || batch.BaseVersion > maxResourceRevision || len(batch.Events) > 1000 {
		return ErrInvalidResource
	}
	feed := ResourceMonitorFeed{ID: monitorFeedID("events"), Kind: "events"}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&feed).Error; err != nil {
		return err
	}
	if err := tx.Model(&feed).Update("kind", "events").Error; err != nil {
		return err
	}
	if err := tx.First(&feed, "id = ?", feed.ID).Error; err != nil {
		return err
	}
	source := resourceMonitorSourceID()
	if batch.Baseline != nil {
		prices := slices.Clone(*batch.Baseline)
		if len(prices) > 10000 {
			return ErrInvalidResource
		}
		slices.SortFunc(prices, func(a, b ResourceDirectoryPrice) int { return compareResourceID(a.ResourceID, b.ResourceID) })
		for i, price := range prices {
			if !resourceText(price.ResourceID, 128, true) || !validPrice(price.PriceTier) || (i > 0 && prices[i-1].ResourceID == price.ResourceID) {
				return ErrInvalidResource
			}
			prices[i].Source, prices[i].Version = source, batch.BaseVersion
		}
		digest := monitorHash(batch.BaseVersion, prices)
		if feed.Initialized {
			if digest != feed.BaselineDigest {
				return ErrResourceConflict
			}
		} else {
			if err := tx.Where("source = ?", source).Delete(&ResourceDirectoryPrice{}).Error; err != nil {
				return err
			}
			if len(prices) > 0 {
				if err := tx.Create(&prices).Error; err != nil {
					return err
				}
			}
			feed.Version, feed.BaselineDigest = batch.BaseVersion, digest
		}
	}
	if batch.BaseVersion > feed.Version {
		return fmt.Errorf("event_version_gap")
	}
	if err := checkpointResourcePriceEvents(tx, &feed, now); err != nil {
		return err
	}
	for _, event := range batch.Events {
		if !resourceText(event.EventID, 128, true) || !resourceText(event.Resource.ResourceID, 128, true) || event.Version <= 0 || event.Version > maxResourceRevision {
			return ErrInvalidResource
		}
		if _, err := time.Parse(time.RFC3339Nano, event.OccurredAt); err != nil {
			return ErrInvalidResource
		}
		if !slices.Contains([]string{"resource.upsert", "resource.disable", "resource.delete", "credential.rotate"}, event.Type) {
			return ErrInvalidResource
		}
		if event.Type == "resource.upsert" && (event.Resource.PriceTier == nil || !validPrice(*event.Resource.PriceTier)) {
			return ErrInvalidResource
		}
		id, digest := monitorHash(source, event.EventID), monitorHash(event)
		var prior ResourceObservedEvent
		err := tx.First(&prior, "id = ? OR (source = ? AND version = ?)", id, source, event.Version).Error
		if err == nil {
			if prior.ID != id || prior.Digest != digest {
				return ErrResourceConflict
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if event.Version <= batch.BaseVersion && event.Version <= feed.Version {
			continue
		}
		if event.Version != feed.Version+1 {
			return fmt.Errorf("event_version_gap")
		}
		if event.Type == "resource.upsert" {
			price := ResourceDirectoryPrice{Source: source, ResourceID: event.Resource.ResourceID, PriceTier: *event.Resource.PriceTier, Version: event.Version}
			if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&price).Error; err != nil {
				return err
			}
		} else if event.Type == "resource.delete" || event.Type == "resource.disable" {
			if err := tx.Where("source = ? AND resource_id = ?", source, event.Resource.ResourceID).Delete(&ResourceDirectoryPrice{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&ResourceObservedEvent{ID: id, Source: source, Version: event.Version, Digest: digest}).Error; err != nil {
			return err
		}
		feed.Version = event.Version
		// Keep each transition inside the batch transaction, including conflicts that recover later.
		if err := checkpointResourcePriceEvents(tx, &feed, now); err != nil {
			return err
		}
	}
	return nil
}

func checkpointResourcePriceEvents(tx *gorm.DB, feed *ResourceMonitorFeed, now time.Time) error {
	feed.Initialized, feed.LastSuccessAt, feed.ErrorCode = true, now.Unix(), ""
	if err := tx.Save(feed).Error; err != nil {
		return err
	}
	return reconcileResourcePrices(tx, now)
}

func ApplyResourcePriceEvents(ctx context.Context, batch ResourcePriceEventBatch, now time.Time) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return applyResourcePriceEvents(tx, batch, now) })
}

func currentDirectoryPrices(tx *gorm.DB) ([]ResourceDirectoryPrice, bool, error) {
	if config.ResourceMonitoringSource().EventsURL == "" {
		return nil, false, nil
	}
	var feed ResourceMonitorFeed
	err := tx.First(&feed, "id = ?", monitorFeedID("events")).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var sync ResourceSyncState
	err = tx.First(&sync, 1).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	if !feed.Initialized || (sync.Source == resourceSourceID() && feed.Version < sync.Version) {
		return nil, false, nil
	}
	var rows []ResourceDirectoryPrice
	err = tx.Where("source = ?", resourceMonitorSourceID()).Find(&rows).Error
	return rows, true, err
}

func overlayResourceDirectoryPrices(tx *gorm.DB, catalog *ResourceCatalog) error {
	prices, ready, err := currentDirectoryPrices(tx)
	if err != nil || !ready || catalog.Snapshot == nil {
		return err
	}
	byID := map[string]int{}
	for _, price := range prices {
		byID[price.ResourceID] = price.PriceTier
	}
	for i := range catalog.Snapshot.Instances {
		if p, ok := byID[catalog.Snapshot.Instances[i].ID]; ok {
			catalog.Snapshot.Instances[i].PriceTier = &p
		}
	}
	return nil
}

func reconcileResourcePrices(tx *gorm.DB, now time.Time) error {
	prices, ready, err := currentDirectoryPrices(tx)
	if err != nil || !ready {
		return err
	}
	catalog, err := readResourceCatalog(tx)
	if err != nil {
		return err
	}
	byID := map[string]ResourceDirectoryPrice{}
	for _, price := range prices {
		byID[price.ResourceID] = price
	}
	active := []string{}
	created := false
	if catalog.Snapshot != nil {
		for _, r := range catalog.Snapshot.Instances {
			price, ok := byID[r.ID]
			if !ok || r.PriceTier == nil || *r.PriceTier == price.PriceTier {
				continue
			}
			row := ResourcePriceMismatch{Source: resourceMonitorSourceID(), ResourceID: r.ID, SnapshotSequence: catalog.Snapshot.Sequence, SnapshotPrice: *r.PriceTier, EventPrice: price.PriceTier, EventVersion: price.Version, DetectedAt: now.Unix()}
			row.ID = monitorHash(row.Source, r.ID, row.SnapshotSequence, row.SnapshotPrice, price.PriceTier, price.Version)
			var ongoing ResourcePriceMismatch
			err := tx.Where("source = ? AND resource_id = ? AND snapshot_price = ? AND event_price = ? AND event_version = ? AND resolved_at = 0", row.Source, row.ResourceID, row.SnapshotPrice, row.EventPrice, row.EventVersion).First(&ongoing).Error
			if err == nil {
				active = append(active, ongoing.ID)
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			active = append(active, row.ID)
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
			if result.Error != nil {
				return result.Error
			}
			created = created || result.RowsAffected > 0
		}
	}
	query := tx.Model(&ResourcePriceMismatch{}).Where("source = ? AND resolved_at = 0", resourceMonitorSourceID())
	if len(active) > 0 {
		query = query.Where("id NOT IN ?", active)
	}
	if err := query.Update("resolved_at", now.Unix()).Error; err != nil {
		return err
	}
	if created {
		return tx.Model(&ResourceSyncState{}).Where("id = 1").Update("next_attempt_at", 0).Error
	}
	return nil
}

type ResourceMonitorHistory struct {
	Mismatches         []ResourcePriceMismatch `json:"mismatches"`
	MismatchTotal      int64                   `json:"mismatch_total"`
	Incidents          []ResourceIncident      `json:"incidents"`
	IncidentTotal      int64                   `json:"incident_total"`
	Feeds              []ResourceMonitorFeed   `json:"feeds"`
	EventsConfigured   bool                    `json:"events_configured"`
	SnapshotConfigured bool                    `json:"snapshot_configured"`
	ReportsConfigured  bool                    `json:"reports_configured"`
}

func ReadResourceMonitorHistory(ctx context.Context, mismatchPage, incidentPage int) (*ResourceMonitorHistory, error) {
	source := config.ResourceMonitoringSource()
	result := &ResourceMonitorHistory{Mismatches: []ResourcePriceMismatch{}, Incidents: []ResourceIncident{}, Feeds: []ResourceMonitorFeed{}, EventsConfigured: source.EventsURL != "", SnapshotConfigured: source.SnapshotURL != "", ReportsConfigured: source.IncidentsURL != ""}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&ResourcePriceMismatch{}).Where("source = ?", resourceMonitorSourceID())
		if err := q.Count(&result.MismatchTotal).Error; err != nil {
			return err
		}
		if err := q.Order("detected_at DESC, event_version DESC, id DESC").Offset((mismatchPage - 1) * 20).Limit(20).Find(&result.Mismatches).Error; err != nil {
			return err
		}
		q = tx.Model(&ResourceIncident{})
		if err := q.Count(&result.IncidentTotal).Error; err != nil {
			return err
		}
		if err := q.Order("occurred_at DESC, id DESC").Offset((incidentPage - 1) * 20).Limit(20).Find(&result.Incidents).Error; err != nil {
			return err
		}
		for i := range result.Incidents {
			row := &result.Incidents[i]
			row.DeliveryStatus = "pending"
			if row.Attempts > 0 {
				row.DeliveryStatus = "retrying"
			}
			if !result.ReportsConfigured || (row.Destination != "" && row.Destination != monitorHash(source.IncidentsURL)) {
				row.DeliveryStatus = "not_configured"
			}
			if row.DeliveredAt > 0 {
				row.DeliveryStatus = "delivered"
			}
		}
		return tx.Where("id IN ?", []string{monitorFeedID("events"), monitorFeedID("snapshot")}).Find(&result.Feeds).Error
	})
	return result, err
}
