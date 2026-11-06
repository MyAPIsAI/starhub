package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"starhub/common"
	"starhub/config"
)

type ResourceIncident struct {
	ID             string `gorm:"primaryKey" json:"event_id"`
	Type           string `json:"type"`
	ResourceID     string `json:"resource_id"`
	GatewayID      string `json:"gateway_id"`
	OccurredAt     int64  `gorm:"index" json:"occurred_at"`
	DeliveredAt    int64  `json:"delivered_at"`
	Attempts       int    `json:"attempts"`
	ErrorCode      string `json:"error_code"`
	Destination    string `json:"-"`
	NextAttemptAt  int64  `gorm:"index" json:"-"`
	Lease          string `json:"-"`
	DeliveryStatus string `gorm:"-" json:"delivery_status"`
}

func newResourceIncident(kind, resourceID, gatewayID string, now time.Time) ResourceIncident {
	destination := ""
	if address := config.ResourceMonitoringSource().IncidentsURL; address != "" {
		destination = monitorHash(address)
	}
	return ResourceIncident{ID: common.GenerateSessionKey(), Type: kind, ResourceID: resourceID, GatewayID: gatewayID, OccurredAt: now.Unix(), Destination: destination}
}

// Delivery is at least once; the payload and idempotency key remain stable across retries.
func DeliverResourceIncidents(ctx context.Context, now time.Time) error {
	source := config.ResourceMonitoringSource()
	if source.IncidentsURL == "" {
		return nil
	}
	destination := monitorHash(source.IncidentsURL)
	var rows []ResourceIncident
	if err := DB.WithContext(ctx).Where("delivered_at = 0 AND next_attempt_at <= ? AND (destination = '' OR destination = ?)", now.Unix(), destination).Order("occurred_at, id").Limit(50).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		lease := common.GenerateSessionKey()
		claim := DB.WithContext(ctx).Model(&ResourceIncident{}).Where("id = ? AND delivered_at = 0 AND next_attempt_at <= ?", row.ID, now.Unix()).Updates(map[string]any{"lease": lease, "next_attempt_at": now.Add(time.Minute).Unix(), "destination": destination, "attempts": gorm.Expr("attempts + 1")})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			continue
		}
		payload, _ := json.Marshal(map[string]any{"event_id": row.ID, "type": row.Type, "resource_id": row.ResourceID, "gateway_id": row.GatewayID, "occurred_at": time.Unix(row.OccurredAt, 0).UTC().Format(time.RFC3339)})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, source.IncidentsURL, bytes.NewReader(payload))
		code := "source_unreachable"
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", row.ID)
			resourceRequestAuth(req)
			response, err := config.ResourceHTTPClient(5 * time.Second).Do(req)
			if err == nil {
				_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
				response.Body.Close()
				code = "source_rejected"
				if response.StatusCode >= 200 && response.StatusCode < 300 {
					code = ""
				}
			}
		}
		updates := map[string]any{"lease": "", "error_code": code, "next_attempt_at": now.Add(time.Duration(min(300, 5<<min(row.Attempts, 6))) * time.Second).Unix()}
		if code == "" {
			updates["delivered_at"] = now.Unix()
		}
		if err := DB.WithContext(ctx).Model(&ResourceIncident{}).Where("id = ? AND lease = ?", row.ID, lease).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

func resourceRequestAuth(request *http.Request) {
	if token := config.ResourceSnapshotSource().Token; token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
}

func fetchResourceObservation(ctx context.Context, address string, target any) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return "invalid_endpoint"
	}
	resourceRequestAuth(req)
	req.Header.Set("Accept", "application/json")
	response, err := config.ResourceHTTPClient(5 * time.Second).Do(req)
	if err != nil {
		return "source_unreachable"
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "source_rejected"
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 || json.Unmarshal(raw, target) != nil {
		return "invalid_payload"
	}
	return ""
}

func pollResourceObservation(ctx context.Context, now time.Time, kind, address string, interval time.Duration) error {
	if address == "" {
		return nil
	}
	feed := ResourceMonitorFeed{ID: monitorFeedID(kind), Kind: kind}
	if err := DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&feed).Error; err != nil {
		return err
	}
	lease := common.GenerateSessionKey()
	claim := DB.WithContext(ctx).Model(&feed).Where("lease_until <= ? AND next_attempt_at <= ?", now.Unix(), now.Unix()).Updates(map[string]any{"lease": lease, "lease_until": now.Add(20 * time.Second).Unix(), "last_attempt_at": now.Unix(), "next_attempt_at": now.Add(interval).Unix()})
	if claim.Error != nil {
		return claim.Error
	}
	if claim.RowsAffected != 1 {
		return nil
	}
	if err := DB.WithContext(ctx).First(&feed, "id = ?", feed.ID).Error; err != nil {
		return err
	}
	var events ResourcePriceEventBatch
	var health struct {
		DirectoryVersion int64            `json:"directory_version"`
		Snapshot         ResourceSnapshot `json:"snapshot"`
	}
	code := ""
	if kind == "events" {
		u, _ := url.Parse(address)
		query := u.Query()
		query.Set("after_version", fmt.Sprint(feed.Version))
		u.RawQuery = query.Encode()
		code = fetchResourceObservation(ctx, u.String(), &events)
	} else {
		code = fetchResourceObservation(ctx, address, &health)
	}
	if code == "" {
		err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			locked := tx.Model(&feed).Where("lease = ?", lease).Update("lease_until", now.Add(20*time.Second).Unix())
			if locked.Error != nil {
				return locked.Error
			}
			if locked.RowsAffected != 1 {
				return ErrResourceSyncBusy
			}
			if kind == "events" {
				return applyResourcePriceEvents(tx, events, now)
			}
			if err := validateResourceSnapshot(health.Snapshot); err != nil {
				return err
			}
			var state ResourceSyncState
			err := tx.First(&state, 1).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if state.Source == resourceSourceID() && health.DirectoryVersion != state.Version {
				return fmt.Errorf("directory_version_gap")
			}
			catalog, err := readResourceCatalog(tx)
			if err != nil {
				return err
			}
			if catalog.Snapshot != nil && health.Snapshot.Sequence <= catalog.Snapshot.Sequence {
				if health.Snapshot.Sequence < catalog.Snapshot.Sequence || health.Snapshot.PublishedAt != catalog.Snapshot.PublishedAt || resourceSnapshotDigest(health.Snapshot, false) != resourceSnapshotDigest(*catalog.Snapshot, false) {
					return ErrResourceConflict
				}
			} else if _, err := saveResourceSnapshot(tx, catalog.Revision, health.Snapshot); err != nil {
				return err
			}
			return reconcileResourcePrices(tx, now)
		})
		if err != nil {
			code = "invalid_payload"
			if errors.Is(err, ErrResourceConflict) {
				code = "version_conflict"
			}
			if err.Error() == "event_version_gap" || err.Error() == "directory_version_gap" {
				code = err.Error()
			}
		}
	}
	updates := map[string]any{"lease": "", "lease_until": 0, "error_code": code}
	if code == "" {
		updates["last_success_at"] = now.Unix()
	}
	return DB.WithContext(ctx).Model(&feed).Where("lease = ?", lease).Updates(updates).Error
}

func PollResourceMonitoring(ctx context.Context, now time.Time) error {
	source := config.ResourceMonitoringSource()
	if err := pollResourceObservation(ctx, now, "events", source.EventsURL, 5*time.Second); err != nil {
		return err
	}
	return pollResourceObservation(ctx, now, "snapshot", source.SnapshotURL, time.Minute)
}

func WatchResourceMonitoring(ctx context.Context) {
	go watchResourceIncidentDelivery(ctx)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := PollResourceMonitoring(ctx, time.Now()); err != nil {
			log.Print("resource monitoring poll failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func watchResourceIncidentDelivery(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := DeliverResourceIncidents(ctx, time.Now()); err != nil {
			log.Print("resource incident delivery persistence failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
