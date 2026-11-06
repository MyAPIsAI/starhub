package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"
	"time"

	"gorm.io/gorm"
	"starhub/common"
	"starhub/config"
)

const ResourceSyncInterval = 5 * time.Minute

var ErrResourceSyncBusy = errors.New("resource synchronization already running")

// Sync state is shared by control-plane processes. The lease fences late writers.
type ResourceSyncState struct {
	ID                int    `gorm:"primaryKey" json:"-"`
	Source            string `json:"-"`
	Mode              string `json:"mode"`
	Version           int64  `json:"applied_version"`
	RemoteVersion     *int64 `json:"remote_version"`
	Checksum          string `json:"checksum"`
	LastAttemptAt     int64  `json:"last_attempt_at"`
	LastAlignedAt     int64  `json:"last_aligned_at"`
	StartedAt         int64  `json:"started_at"`
	FailedCycles      int    `json:"failed_cycles"`
	LastFailedCycleAt int64  `json:"-"`
	Lease             string `json:"-"`
	LeaseUntil        int64  `json:"-"`
	NextAttemptAt     int64  `json:"next_attempt_at"`
}

type ResourceSyncFailure struct {
	ID     int64  `gorm:"primaryKey" json:"id"`
	Source string `gorm:"index" json:"-"`
	At     int64  `json:"at"`
	Code   string `json:"code"`
}

type ResourceSyncView struct {
	ResourceSyncState
	Configured     bool                   `json:"configured"`
	Status         string                 `json:"status"`
	VersionGap     *int64                 `json:"version_gap"`
	Revision       int64                  `json:"revision"`
	Sequence       int64                  `json:"sequence"`
	Failures       []ResourceSyncFailure  `json:"failures"`
	ObservedAt     int64                  `json:"observed_at"`
	SnapshotStatus ResourceSnapshotStatus `json:"snapshot"`
}

func resourceSourceID() string {
	source := config.ResourceSnapshotSource()
	sum := sha256.Sum256([]byte(source.URL + "\x00" + source.Token))
	return hex.EncodeToString(sum[:])
}

// Hash the public inventory only, independently of ordering and receipt time.
func ResourceSnapshotChecksum(snapshot ResourceSnapshot) string {
	return resourceSnapshotDigest(snapshot, true)
}

func resourceSnapshotDigest(snapshot ResourceSnapshot, directoryOnly bool) string {
	snapshot.ReceivedAt, snapshot.Source, snapshot.PublishedAt = "", "", ""
	snapshot.ID, snapshot.Sequence = "", 0
	snapshot.Channels = slices.Clone(snapshot.Channels)
	snapshot.Instances = slices.Clone(snapshot.Instances)
	slices.SortFunc(snapshot.Channels, func(a, b ResourceChannel) int { return compareResourceID(a.ID, b.ID) })
	slices.SortFunc(snapshot.Instances, func(a, b ResourceInstance) int { return compareResourceID(a.ID, b.ID) })
	for i := range snapshot.Instances {
		r := &snapshot.Instances[i]
		if directoryOnly {
			r.QualityLevel, r.State, r.Trend, r.Score, r.LatencyMS = "", "", "", nil, 0
		}
		r.Protocols, r.Capabilities = slices.Clone(r.Protocols), slices.Clone(r.Capabilities)
		slices.Sort(r.Protocols)
		slices.Sort(r.Capabilities)
	}
	raw, _ := json.Marshal(snapshot)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func compareResourceID(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func ReadResourceSyncStatus(ctx context.Context, now time.Time) (*ResourceSyncView, error) {
	view := &ResourceSyncView{Configured: config.ResourceSnapshotSource().URL != "", Status: "not_configured", Failures: []ResourceSyncFailure{}, ObservedAt: now.Unix()}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		catalog, err := readResourceCatalog(tx)
		if err != nil {
			return err
		}
		view.Revision = catalog.Revision
		view.SnapshotStatus = resourceSnapshotStatus(catalog.Snapshot, now)
		if catalog.Snapshot != nil {
			view.Sequence = catalog.Snapshot.Sequence
		}
		var state ResourceSyncState
		err = tx.First(&state, 1).Error
		if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && state.Source != resourceSourceID()) {
			if view.Configured {
				view.Status = "awaiting_sync"
			}
			return nil
		}
		if err != nil {
			return err
		}
		view.ResourceSyncState = state
		if err := tx.Where("source = ?", state.Source).Order("id DESC").Limit(20).Find(&view.Failures).Error; err != nil {
			return err
		}
		if !view.Configured {
			return nil
		}
		view.Status = "aligned"
		if state.LastAlignedAt == 0 {
			view.Status = "awaiting_sync"
		}
		if state.FailedCycles > 0 {
			view.Status = "retrying"
		}
		if state.RemoteVersion != nil {
			gap := *state.RemoteVersion - state.Version
			view.VersionGap = &gap
		}
		baseline := state.LastAlignedAt
		if baseline == 0 {
			baseline = state.StartedAt
		}
		if state.FailedCycles >= 2 || now.Unix()-baseline >= int64((2*ResourceSyncInterval).Seconds()) {
			view.Status = "alert"
		}
		if catalog.Snapshot != nil && state.Checksum != "" && ResourceSnapshotChecksum(*catalog.Snapshot) != state.Checksum {
			view.Status = "alert"
		}
		if state.LeaseUntil > now.Unix() && view.Status != "alert" {
			view.Status = "syncing"
		}
		return nil
	})
	return view, err
}

// Both scheduled and manual runs use this boundary. Only manual runs carry an actor.
func SyncResourcesOnce(ctx context.Context, now time.Time, actor int64) (*ResourceSyncView, error) {
	source := config.ResourceSnapshotSource()
	if source.URL == "" {
		return nil, errors.New("resource snapshot source is not configured")
	}
	sourceID, lease := resourceSourceID(), common.GenerateSessionKey()
	var state ResourceSyncState
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.FirstOrCreate(&ResourceSyncState{ID: 1}, "id = 1").Error; err != nil {
			return err
		}
		claim := tx.Model(&ResourceSyncState{}).Where("id = 1 AND lease_until <= ?", now.Unix())
		if actor == 0 {
			claim = claim.Where("source <> ? OR next_attempt_at <= ?", sourceID, now.Unix())
		}
		res := claim.Updates(map[string]any{"lease": lease, "lease_until": now.Add(30 * time.Second).Unix()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrResourceSyncBusy
		}
		if err := tx.First(&state, 1).Error; err != nil {
			return err
		}
		if state.Source != sourceID {
			state = ResourceSyncState{ID: 1, Source: sourceID, StartedAt: now.Unix(), Lease: lease, LeaseUntil: now.Add(30 * time.Second).Unix()}
		}
		state.LastAttemptAt, state.NextAttemptAt = now.Unix(), now.Add(ResourceSyncInterval).Unix()
		return tx.Save(&state).Error
	})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	snapshot, version, mode, code := fetchResourceSnapshot(ctx, source, state)
	if code == "" {
		previous := state
		err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			res := tx.Model(&ResourceSyncState{}).Where("id = 1 AND lease = ? AND source = ?", lease, sourceID).Update("lease_until", now.Unix())
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return ErrResourceSyncBusy
			}
			catalog, err := readResourceCatalog(tx)
			if err != nil {
				return err
			}
			digest := ResourceSnapshotChecksum(snapshot)
			if version < state.Version || (version == state.Version && state.Checksum != "" && digest != state.Checksum) {
				return ErrResourceConflict
			}
			if catalog.Snapshot != nil && snapshot.Sequence == catalog.Snapshot.Sequence {
				if resourceSnapshotDigest(*catalog.Snapshot, false) != resourceSnapshotDigest(snapshot, false) {
					return ErrResourceConflict
				}
			} else if _, err := saveResourceSnapshot(tx, catalog.Revision, snapshot); err != nil {
				return err
			}
			before := map[string]any{"version": state.Version, "checksum": state.Checksum}
			state.Version, state.RemoteVersion, state.Checksum, state.Mode = version, &version, digest, mode
			state.LastAlignedAt, state.FailedCycles, state.LastFailedCycleAt = now.Unix(), 0, 0
			state.Lease, state.LeaseUntil = "", 0
			if err := tx.Save(&state).Error; err != nil {
				return err
			}
			if err := reconcileResourcePrices(tx, now); err != nil {
				return err
			}
			if actor != 0 {
				return RecordAdminChange(tx, actor, "resource_sync", "resource-sync", "1", before, map[string]any{"version": version, "checksum": digest, "sequence": snapshot.Sequence}, "Forced full synchronization", "")
			}
			return nil
		})
		if err != nil {
			state = previous
			code = "apply_failed"
			if errors.Is(err, ErrResourceConflict) {
				code = "version_conflict"
			}
		}
	}
	if code != "" {
		// Failure recording survives caller cancellation and never stores upstream messages or URLs.
		failureCtx, failureCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer failureCancel()
		err := DB.WithContext(failureCtx).Transaction(func(tx *gorm.DB) error {
			res := tx.Model(&ResourceSyncState{}).Where("id = 1 AND lease = ?", lease).Update("lease_until", 0)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return ErrResourceSyncBusy
			}
			if state.LastFailedCycleAt == 0 || now.Unix()-state.LastFailedCycleAt >= int64(ResourceSyncInterval.Seconds()) {
				state.FailedCycles++
				state.LastFailedCycleAt = now.Unix()
			}
			if version > 0 {
				state.RemoteVersion = &version
			}
			state.Lease, state.LeaseUntil = "", 0
			if err := tx.Save(&state).Error; err != nil {
				return err
			}
			if err := tx.Create(&ResourceSyncFailure{Source: sourceID, At: now.Unix(), Code: code}).Error; err != nil {
				return err
			}
			return tx.Exec("DELETE FROM resource_sync_failures WHERE id NOT IN (SELECT id FROM resource_sync_failures ORDER BY id DESC LIMIT 100)").Error
		})
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("resource synchronization: %s", code)
	}
	return ReadResourceSyncStatus(ctx, now)
}

// The versioned envelope keeps directory version distinct from snapshot sequence.
// The legacy snapshot endpoint remains supported and identifies its compatibility mode.
func fetchResourceSnapshot(ctx context.Context, source config.SnapshotSource, state ResourceSyncState) (ResourceSnapshot, int64, string, string) {
	var snapshot ResourceSnapshot
	address, err := url.Parse(source.URL)
	if err != nil {
		return snapshot, 0, "", "source_invalid"
	}
	query := address.Query()
	query.Set("full", "true")
	query.Set("version", fmt.Sprint(state.Version))
	query.Set("checksum", state.Checksum)
	address.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address.String(), nil)
	if err != nil {
		return snapshot, 0, "", "source_invalid"
	}
	req.Header.Set("Accept", "application/json")
	if source.Token != "" {
		req.Header.Set("Authorization", "Bearer "+source.Token)
	}
	client := config.ResourceHTTPClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return snapshot, 0, "", "source_unreachable"
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return snapshot, 0, "", "source_rejected"
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return snapshot, 0, "", "invalid_payload"
	}
	var envelope struct {
		Version  int64             `json:"version"`
		Checksum string            `json:"checksum"`
		Snapshot *ResourceSnapshot `json:"snapshot"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return snapshot, 0, "", "invalid_payload"
	}
	version, mode := int64(0), "versioned_snapshot"
	if envelope.Snapshot != nil {
		snapshot, version = *envelope.Snapshot, envelope.Version
		if version <= 0 || version > maxResourceRevision {
			return snapshot, 0, mode, "invalid_payload"
		}
		if envelope.Checksum != ResourceSnapshotChecksum(snapshot) {
			return snapshot, version, mode, "checksum_mismatch"
		}
	} else {
		if json.Unmarshal(raw, &snapshot) != nil {
			return snapshot, 0, "", "invalid_payload"
		}
		version, mode = snapshot.Sequence, "snapshot_compatibility"
	}
	if validateResourceSnapshot(snapshot) != nil {
		return snapshot, version, mode, "invalid_snapshot"
	}
	return snapshot, version, mode, ""
}

func WatchResourceSync(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if config.ResourceSnapshotSource().URL != "" {
			if _, err := SyncResourcesOnce(ctx, time.Now(), 0); err != nil && !errors.Is(err, ErrResourceSyncBusy) {
				log.Printf("resource sync: %v", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
