package model

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"
)

// RouteSnapshot retains the publisher's exact JSON field order for checksums.
// Unknown evidence fields remain covered by the checksum without being discarded.
type RouteSnapshot struct {
	Policy                 json.RawMessage   `json:"policy"`
	PolicyFingerprint      string            `json:"policy_fingerprint"`
	SnapshotVersion        string            `json:"snapshot_version"`
	ScoreAlgorithmVersion  string            `json:"score_algorithm_version"`
	RatingAlgorithmVersion string            `json:"rating_algorithm_version"`
	PublishedAt            time.Time         `json:"published_at"`
	Checksum               string            `json:"checksum"`
	ResourceCount          int               `json:"resource_count"`
	EligibleCount          int               `json:"eligible_count"`
	Items                  []json.RawMessage `json:"items"`
}

type RouteProtection struct {
	Schema             int    `json:"schema"`
	CatalogExcluded    bool   `json:"catalog_excluded"`
	EndpointGeneration string `json:"endpoint_generation"`
	RecoveryEpoch      int64  `json:"recovery_epoch"`
	StateVersion       int64  `json:"state_version"`
	Mode               string `json:"mode"`
}

type RouteGroupPrice struct {
	Group      string `json:"group"`
	Ratio      string `json:"ratio"`
	BaseCost   string `json:"base_cost"`
	ActualCost string `json:"actual_cost"`
	Status     string `json:"status"`
	Error      string `json:"error"`
}

type RouteItem struct {
	ResourceID   string             `json:"resource_id"`
	ChannelID    string             `json:"channel_id"`
	ModelID      string             `json:"model_id"`
	ModelName    string             `json:"model_name"`
	PriceTier    int                `json:"price_tier"`
	QualityLevel string             `json:"quality_level"`
	State        string             `json:"state"`
	Trend        string             `json:"trend"`
	Score        *float64           `json:"score"`
	DimScores    map[string]float64 `json:"dim_scores"`
	Protection   *RouteProtection   `json:"protection"`
	Pricing      *struct {
		SchemaVersion int               `json:"schema_version"`
		CostBasis     string            `json:"cost_basis"`
		Groups        []RouteGroupPrice `json:"groups"`
	} `json:"pricing"`
	ScoreEvidence *struct {
		Lifecycle *struct {
			Admission string `json:"admission"`
		} `json:"lifecycle"`
	} `json:"score_evidence"`
}

type RoutingSnapshotRecord struct {
	ID          int    `gorm:"primaryKey"`
	Payload     string `gorm:"type:text;not null"`
	Version     string
	Checksum    string
	PublishedAt time.Time
	ReceivedAt  time.Time
}

var routingSnapshotState struct {
	sync.RWMutex
	snapshot *RouteSnapshot
	items    []RouteItem
}

func ResetRoutingSnapshot() {
	routingSnapshotState.Lock()
	defer routingSnapshotState.Unlock()
	routingSnapshotState.snapshot = nil
	routingSnapshotState.items = nil
}

func (s RouteSnapshot) PolicyChecksum() string {
	items := append([]json.RawMessage{}, s.Items...)
	sort.SliceStable(items, func(i, j int) bool {
		var a, b RouteItem
		_ = json.Unmarshal(items[i], &a)
		_ = json.Unmarshal(items[j], &b)
		return a.ResourceID < b.ResourceID
	})
	value := struct {
		Format      string            `json:"format"`
		Rating      string            `json:"rating_algorithm_version"`
		Score       string            `json:"score_algorithm_version"`
		Policy      json.RawMessage   `json:"policy"`
		Fingerprint string            `json:"policy_fingerprint"`
		Items       []json.RawMessage `json:"items"`
	}{"checksum-un-in-v1", s.RatingAlgorithmVersion, s.ScoreAlgorithmVersion, s.Policy, s.PolicyFingerprint, items}
	b, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(b))
}

func (s RouteSnapshot) Validate(now time.Time) ([]RouteItem, error) {
	if s.SnapshotVersion == "" || s.ScoreAlgorithmVersion == "" || s.RatingAlgorithmVersion == "" || s.PublishedAt.IsZero() || s.PublishedAt.After(now.Add(time.Minute)) || s.ResourceCount != len(s.Items) || s.EligibleCount < 0 || s.EligibleCount > s.ResourceCount {
		return nil, errors.New("invalid snapshot envelope")
	}
	if s.Checksum == "" || s.Checksum != s.PolicyChecksum() {
		return nil, errors.New("snapshot checksum mismatch")
	}
	items := make([]RouteItem, 0, len(s.Items))
	seen := map[string]bool{}
	for _, raw := range s.Items {
		var item RouteItem
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, errors.New("invalid snapshot item")
		}
		id, err := strconv.ParseInt(item.ChannelID, 10, 64)
		if err != nil || id <= 0 || item.ResourceID == "" || item.ModelID == "" || item.ModelName == "" || seen[item.ResourceID] || item.Score == nil || !finiteScore(*item.Score) {
			return nil, errors.New("invalid resource identity or score")
		}
		seen[item.ResourceID] = true
		if item.ResourceID != fmt.Sprintf("%s:%s:t%d", item.ChannelID, item.ModelID, item.PriceTier) {
			return nil, errors.New("resource identity mismatch")
		}
		switch item.State {
		case "hot", "cold", "cold_new", "cold_silent", "degraded", "disabled", "bad":
		default:
			return nil, errors.New("invalid resource state")
		}
		switch item.Trend {
		case "stable", "degraded", "degrading":
		default:
			return nil, errors.New("invalid resource trend")
		}
		switch item.QualityLevel {
		case "S0", "S1", "S2", "UN", "IN":
		default:
			return nil, errors.New("invalid resource quality")
		}
		for _, key := range []string{"success_rate", "cost", "speed", "health", "stability"} {
			v, ok := item.DimScores[key]
			if !ok || !finiteScore(v) {
				return nil, errors.New("invalid dimension score")
			}
		}
		if p := item.Protection; p != nil {
			if p.Schema != 1 || p.EndpointGeneration == "" || p.RecoveryEpoch < 1 || p.StateVersion < 1 {
				return nil, errors.New("invalid protection state")
			}
			switch p.Mode {
			case "allowed", "review", "disabled", "recovering", "manual":
			default:
				return nil, errors.New("invalid protection mode")
			}
		}
		items = append(items, item)
	}
	return items, nil
}

func finiteScore(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 100 }

// ApplyRoutingSnapshot commits before publishing; duplicates never refresh age.
func ApplyRoutingSnapshot(s RouteSnapshot, now time.Time) error {
	items, err := s.Validate(now)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(s)
	if err != nil {
		return err
	}
	routingSnapshotState.Lock()
	defer routingSnapshotState.Unlock()
	err = DB.Transaction(func(tx *gorm.DB) error {
		var old RoutingSnapshotRecord
		err := tx.First(&old, "id = 1").Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			if old.Version == s.SnapshotVersion {
				if old.Checksum != s.Checksum || !old.PublishedAt.Equal(s.PublishedAt) {
					return errors.New("conflicting snapshot version")
				}
				return nil
			}
			if !s.PublishedAt.After(old.PublishedAt) {
				return errors.New("out of order snapshot")
			}
		}
		return tx.Save(&RoutingSnapshotRecord{ID: 1, Payload: string(payload), Version: s.SnapshotVersion, Checksum: s.Checksum, PublishedAt: s.PublishedAt, ReceivedAt: now}).Error
	})
	if err == nil {
		// Decode a private copy so caller-owned buffers cannot mutate live routing.
		var stored RouteSnapshot
		_ = json.Unmarshal(payload, &stored)
		routingSnapshotState.snapshot = &stored
		routingSnapshotState.items = items
	}
	return err
}

func RoutingSnapshotStatus() (version string, age time.Duration, ready bool) {
	routingSnapshotState.RLock()
	defer routingSnapshotState.RUnlock()
	if s := routingSnapshotState.snapshot; s != nil {
		return s.SnapshotVersion, time.Since(s.PublishedAt), true
	}
	return "", 0, false
}
