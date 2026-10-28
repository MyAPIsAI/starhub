package model

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Binding changes are durable receipts, separate from immutable publication plans.
type ResourceGatewayBindingChange struct {
	ID                      int64  `gorm:"primaryKey;autoIncrement"`
	RequestID               string `gorm:"uniqueIndex;size:32;not null"`
	ActorID                 int64  `gorm:"not null"`
	Operation               string `gorm:"size:16;not null"`
	ChannelID               int64  `gorm:"not null"`
	SourceChannelID         string `gorm:"size:128;not null"`
	RetainedSourceChannelID string `gorm:"size:128;not null"`
	PublicationID           int64  `gorm:"not null"`
	Profile                 string `gorm:"size:64;not null"`
	WasConfiguredActive     bool   `gorm:"not null"`
	RemovedRouteCount       int64  `gorm:"not null"`
	RemovedAbilityCount     int64  `gorm:"not null"`
	ResourceRevision        int64  `gorm:"not null"`
	ChannelRevision         int64  `gorm:"not null"`
	CreatedAt               int64  `gorm:"autoCreateTime"`
}

type ResourceBindingChangeRequest struct {
	ResourceRevision int64
	ChannelRevision  int64
	SourceChannelID  string
	PublicationID    string
	Operation        string
	RequestID        string
}

type ResourceBindingChangeReceipt struct {
	Schema                   string `json:"schema"`
	ChangeID                 string `json:"change_id"`
	RequestID                string `json:"request_id"`
	ActorID                  string `json:"actor_id"`
	Operation                string `json:"operation"`
	GatewayChannelID         string `json:"gateway_channel_id"`
	SourceChannelID          string `json:"source_channel_id"`
	RetainedSourceChannelID  string `json:"retained_source_channel_id"`
	PublicationID            string `json:"publication_id"`
	Profile                  string `json:"profile"`
	WasConfiguredActive      bool   `json:"was_configured_active"`
	RemovedRouteCount        int64  `json:"removed_route_count"`
	RemovedAbilityCount      int64  `json:"removed_ability_count"`
	ResourceRevision         int64  `json:"resource_revision"`
	ChannelRevision          int64  `json:"channel_revision"`
	CreatedAt                int64  `json:"created_at"`
	Historical               bool   `json:"historical"`
	Committed                bool   `json:"committed"`
	ProtectiveMarkerRetained bool   `json:"protective_marker_retained"`
}

var ErrResourceBindingChangeNotFound = errors.New("binding change receipt not found")

func validBindingRequestID(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 16 && len(value) == 32 && strings.ToLower(value) == value
}

func resourceBindingChangeReceipt(row ResourceGatewayBindingChange) (*ResourceBindingChangeReceipt, error) {
	if row.ID <= 0 || row.ActorID <= 0 || row.ChannelID <= 0 || row.PublicationID < 0 || !validBindingRequestID(row.RequestID) || !resourceText(row.SourceChannelID, 128, true) || !resourceText(row.Profile, 64, false) || (row.Operation != "revoke" && row.Operation != "release") || (row.Operation == "revoke" && row.RetainedSourceChannelID != row.SourceChannelID) || (row.Operation == "release" && row.RetainedSourceChannelID != "") || row.RemovedRouteCount < 0 || row.RemovedRouteCount > maxResourceRevision || row.RemovedAbilityCount < 0 || row.RemovedAbilityCount > maxResourceRevision || row.ResourceRevision < 1 || row.ResourceRevision > maxResourceRevision || row.ChannelRevision < 1 || row.ChannelRevision > maxResourceRevision || row.CreatedAt <= 0 || row.CreatedAt > maxResourceRevision {
		return nil, fmt.Errorf("binding change receipt is unavailable")
	}
	return &ResourceBindingChangeReceipt{Schema: "managed_binding_change_v1", ChangeID: strconv.FormatInt(row.ID, 10), RequestID: row.RequestID, ActorID: strconv.FormatInt(row.ActorID, 10), Operation: row.Operation, GatewayChannelID: strconv.FormatInt(row.ChannelID, 10), SourceChannelID: row.SourceChannelID, RetainedSourceChannelID: row.RetainedSourceChannelID, PublicationID: strconv.FormatInt(row.PublicationID, 10), Profile: row.Profile, WasConfiguredActive: row.WasConfiguredActive, RemovedRouteCount: row.RemovedRouteCount, RemovedAbilityCount: row.RemovedAbilityCount, ResourceRevision: row.ResourceRevision, ChannelRevision: row.ChannelRevision, CreatedAt: row.CreatedAt, Historical: true, Committed: true, ProtectiveMarkerRetained: true}, nil
}

func ReadResourceBindingChange(ctx context.Context, requestID string) (*ResourceBindingChangeReceipt, error) {
	if !validBindingRequestID(requestID) {
		return nil, ErrInvalidResource
	}
	var row ResourceGatewayBindingChange
	if err := DB.WithContext(ctx).Where("request_id = ?", requestID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrResourceBindingChangeNotFound
		}
		return nil, err
	}
	return resourceBindingChangeReceipt(row)
}

func ChangeResourceBinding(ctx context.Context, channelID string, request ResourceBindingChangeRequest, actorID int64) (*ResourceBindingChangeReceipt, error) {
	id, err := strconv.ParseInt(channelID, 10, 64)
	publicationID, pubErr := strconv.ParseInt(request.PublicationID, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != channelID || pubErr != nil || publicationID < 0 || strconv.FormatInt(publicationID, 10) != request.PublicationID || actorID <= 0 || !validBindingRequestID(request.RequestID) || !resourceText(request.SourceChannelID, 128, true) || (request.Operation != "revoke" && request.Operation != "release") || request.ResourceRevision < 0 || request.ResourceRevision >= maxResourceRevision || request.ChannelRevision < 0 || request.ChannelRevision >= maxResourceRevision {
		return nil, ErrInvalidResource
	}
	var result *ResourceBindingChangeReceipt
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Acquire the writer before reading either version or current ownership.
		claim := tx.Model(&ResourceCatalogRecord{}).Where("id = 1 AND revision = ?", request.ResourceRevision).UpdateColumn("revision", gorm.Expr("revision"))
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return ErrResourceConflict
		}
		var revision ChannelConfigurationState
		if err := tx.First(&revision, 1).Error; err != nil {
			return err
		}
		if revision.Revision != request.ChannelRevision {
			return ErrResourceConflict
		}
		var existing int64
		if err := tx.Model(&ResourceGatewayBindingChange{}).Where("request_id = ?", request.RequestID).Count(&existing).Error; err != nil {
			return err
		}
		if existing != 0 {
			return ErrResourceConflict
		}
		var owner ResourceGatewayManagedChannel
		if err := tx.First(&owner, "channel_id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrResourceConflict
			}
			return err
		}
		if owner.SourceChannelID != request.SourceChannelID || owner.PublicationID != publicationID {
			return ErrResourceConflict
		}
		row := ResourceGatewayBindingChange{RequestID: request.RequestID, ActorID: actorID, Operation: request.Operation, ChannelID: id, SourceChannelID: owner.SourceChannelID, RetainedSourceChannelID: owner.SourceChannelID, PublicationID: owner.PublicationID, Profile: owner.Profile, WasConfiguredActive: owner.Active, ResourceRevision: request.ResourceRevision + 1, ChannelRevision: request.ChannelRevision + 1, CreatedAt: time.Now().Unix()}
		if request.Operation == "release" {
			row.RetainedSourceChannelID = ""
		}
		if err := tx.Model(&ResourceGatewayRoute{}).Where("channel_id = ?", id).Count(&row.RemovedRouteCount).Error; err != nil {
			return err
		}
		if err := tx.Model(&Ability{}).Where("channel_id = ?", id).Count(&row.RemovedAbilityCount).Error; err != nil {
			return err
		}
		if !resourceText(row.Profile, 64, false) {
			return fmt.Errorf("binding profile is unavailable")
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if err := tx.Model(&ResourceGatewayManagedChannel{}).Where("channel_id = ?", id).Updates(map[string]any{"active": false, "source_channel_id": row.RetainedSourceChannelID}).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", id).Delete(&ResourceGatewayRoute{}).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", id).Delete(&Ability{}).Error; err != nil {
			return err
		}
		if err := advanceChannelConfigurationRevision(tx); err != nil {
			return err
		}
		if err := tx.Model(&ResourceCatalogRecord{}).Where("id = 1").Updates(map[string]any{"revision": row.ResourceRevision, "updated_at": time.Now().Unix()}).Error; err != nil {
			return err
		}
		result, err = resourceBindingChangeReceipt(row)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
