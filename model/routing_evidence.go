package model

import (
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"starhub/common"
)

type RoutingEvidence struct {
	Instances map[string]ResourceInstance
	Sequence  int64
	Revision  int64
	Resources map[string][]string
}

func CaptureRoutingEvidence(ctx context.Context) (*RoutingEvidence, error) {
	result := &RoutingEvidence{Resources: map[string][]string{}, Instances: map[string]ResourceInstance{}}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record ResourceCatalogRecord
		if err := tx.First(&record, 1).Error; err != nil {
			return err
		}
		result.Revision = record.Revision
		if record.Snapshot == "" {
			return nil
		}
		var snapshot ResourceSnapshot
		if err := json.Unmarshal([]byte(record.Snapshot), &snapshot); err != nil {
			return err
		}
		if err := overlayResourceDirectoryPrices(tx, &ResourceCatalog{Snapshot: &snapshot}); err != nil {
			return err
		}
		result.Sequence = snapshot.Sequence
		for _, instance := range snapshot.Instances {
			result.Instances[instance.ID] = instance
		}
		var owners []ResourceGatewayManagedChannel
		if err := tx.Where("active = ?", true).Find(&owners).Error; err != nil {
			return err
		}
		var placements []ResourcePlacementRecord
		if err := tx.Find(&placements).Error; err != nil {
			return err
		}
		mapped := map[string]ResourcePlacementRecord{}
		for _, p := range placements {
			mapped[p.ResourceID] = p
		}
		for _, owner := range owners {
			for _, instance := range snapshot.Instances {
				p, ok := mapped[instance.ID]
				if !ok || p.ModelID == nil || !p.Enabled || instance.ChannelID != owner.SourceChannelID {
					continue
				}
				var groups []string
				if err := json.Unmarshal([]byte(p.GroupIDs), &groups); err != nil {
					return err
				}
				for _, g := range groups {
					key := fmt.Sprintf("%d|%s|%s", owner.ChannelID, *p.ModelID, g)
					result.Resources[key] = append(result.Resources[key], instance.ID)
				}
			}
		}
		return nil
	})
	return result, err
}
func FreezeRoutingEvidence(ctx context.Context, entry *Log, affinity bool) {
	evidence, _ := ctx.Value(common.CtxAdmissionRouting).(*RoutingEvidence)
	if evidence == nil {
		return
	}
	entry.SnapshotSequence = evidence.Sequence
	entry.ResourceRevision = evidence.Revision
	entry.AffinityUsed = affinity
	entry.RoutingRecorded = true
	ids := evidence.Resources[fmt.Sprintf("%d|%s|%s", entry.ChannelID, entry.Model, entry.ServiceGroup)]
	if len(ids) == 1 {
		entry.ResourceID = ids[0]
	}
	if len(ids) > 0 {
		raw, _ := json.Marshal(ids)
		entry.ResourceIDs = string(raw)
	}
}

type taskRoutingEvidence struct {
	ResourceID       string
	ResourceIDs      string
	SnapshotSequence int64
	ResourceRevision int64
	AffinityUsed     bool
	RoutingRecorded  bool
}

func FreezeTaskRouting(ctx context.Context, task *Task, affinity bool) {
	entry := &Log{ChannelID: task.ChannelID, Model: task.Model, ServiceGroup: task.Group}
	FreezeRoutingEvidence(ctx, entry, affinity)
	raw, _ := json.Marshal(taskRoutingEvidence{entry.ResourceID, entry.ResourceIDs, entry.SnapshotSequence, entry.ResourceRevision, entry.AffinityUsed, entry.RoutingRecorded})
	task.RoutingEvidence = string(raw)
}

func ApplyTaskRouting(task *Task, entry *Log) {
	var evidence taskRoutingEvidence
	if json.Unmarshal([]byte(task.RoutingEvidence), &evidence) != nil {
		return
	}
	entry.ResourceID, entry.ResourceIDs = evidence.ResourceID, evidence.ResourceIDs
	entry.SnapshotSequence, entry.ResourceRevision = evidence.SnapshotSequence, evidence.ResourceRevision
	entry.AffinityUsed, entry.RoutingRecorded = evidence.AffinityUsed, evidence.RoutingRecorded
}
