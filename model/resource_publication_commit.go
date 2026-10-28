package model

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Publication records commit with bindings and routes, independently of best-effort HTTP audit.
type ResourceGatewayPublication struct {
	ID               int64  `gorm:"primaryKey;autoIncrement"`
	ActorID          int64  `gorm:"not null"`
	Operation        string `gorm:"size:32;not null"`
	Profile          string `gorm:"size:64;not null"`
	ReviewDigest     string `gorm:"uniqueIndex;size:64;not null"`
	ResourceRevision int64  `gorm:"not null"`
	ChannelRevision  int64  `gorm:"not null"`
	OptionVersion    int64  `gorm:"not null"`
	Bindings         string `gorm:"type:text;not null"`
	Routes           string `gorm:"type:text;not null"`
	CreatedAt        int64  `gorm:"autoCreateTime"`
}

type ResourcePublicationAcknowledgement struct {
	PublicationID    string `json:"publication_id"`
	ReviewDigest     string `json:"review_digest"`
	ResourceRevision int64  `json:"resource_revision"`
	ChannelRevision  int64  `json:"channel_revision"`
	OptionVersion    int64  `json:"option_version"`
	Profile          string `json:"profile"`
	BindingCount     int    `json:"binding_count"`
	RouteCount       int    `json:"route_count"`
	Published        bool   `json:"published"`
	Operation        string `json:"operation"`
}

var ErrResourcePublicationNotFound = errors.New("publication receipt not found")

type ResourcePublicationReceipt struct {
	ResourcePublicationAcknowledgement
	ActorID    string                     `json:"actor_id"`
	CreatedAt  int64                      `json:"created_at"`
	Bindings   []ResourceChannelBinding   `json:"bindings"`
	Routes     []ResourcePublicationRoute `json:"routes"`
	Historical bool                       `json:"historical"`
	Committed  bool                       `json:"committed"`
}

func ReadResourcePublicationReceipt(ctx context.Context, digest string) (*ResourcePublicationReceipt, error) {
	if raw, err := hex.DecodeString(digest); err != nil || len(raw) != 32 || len(digest) != 64 {
		return nil, ErrInvalidResource
	}
	var receipt ResourceGatewayPublication
	if err := DB.WithContext(ctx).Where("review_digest = ?", digest).First(&receipt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrResourcePublicationNotFound
		}
		return nil, err
	}
	var bindings []ResourceChannelBinding
	var routes []ResourcePublicationRoute
	if json.Unmarshal([]byte(receipt.Bindings), &bindings) != nil || json.Unmarshal([]byte(receipt.Routes), &routes) != nil || receipt.Operation != "publish" || receipt.Profile != ResourceGatewayTextProfile || len(bindings) == 0 || len(bindings) > 500 || len(routes) == 0 || len(routes) > maxResourcePublicationPairs || receipt.ActorID <= 0 || receipt.ID <= 0 || receipt.ResourceRevision < 1 || receipt.ResourceRevision > maxResourceRevision || receipt.ChannelRevision < 1 || receipt.ChannelRevision > maxResourceRevision || receipt.OptionVersion < 0 || receipt.OptionVersion > maxResourceRevision || receipt.CreatedAt <= 0 || receipt.CreatedAt > maxResourceRevision || !validResourcePublicationReceiptRoutes(bindings, routes) {
		return nil, fmt.Errorf("publication receipt is unavailable")
	}
	return &ResourcePublicationReceipt{ResourcePublicationAcknowledgement: ResourcePublicationAcknowledgement{PublicationID: strconv.FormatInt(receipt.ID, 10), ReviewDigest: receipt.ReviewDigest, ResourceRevision: receipt.ResourceRevision, ChannelRevision: receipt.ChannelRevision, OptionVersion: receipt.OptionVersion, Profile: receipt.Profile, BindingCount: len(bindings), RouteCount: len(routes), Published: true, Operation: receipt.Operation}, ActorID: strconv.FormatInt(receipt.ActorID, 10), CreatedAt: receipt.CreatedAt, Bindings: bindings, Routes: routes, Historical: true, Committed: true}, nil
}

func validResourcePublicationReceiptRoutes(bindings []ResourceChannelBinding, routes []ResourcePublicationRoute) bool {
	targets := map[string]bool{}
	sources := map[string]bool{}
	for _, binding := range bindings {
		id, err := strconv.ParseInt(binding.GatewayChannelID, 10, 64)
		if err != nil || id <= 0 || strconv.FormatInt(id, 10) != binding.GatewayChannelID || !resourceText(binding.SourceChannelID, 128, true) || sources[binding.SourceChannelID] || targets[binding.GatewayChannelID] {
			return false
		}
		targets[binding.GatewayChannelID] = true
		sources[binding.SourceChannelID] = true
	}
	pairs := map[ResourcePublicationPair]bool{}
	used := map[string]bool{}
	for _, route := range routes {
		groupID, err := strconv.ParseInt(route.GroupDefinitionID, 10, 64)
		if err != nil || groupID < 0 || strconv.FormatInt(groupID, 10) != route.GroupDefinitionID || !targets[route.GatewayChannelID] || !resourceText(route.Model, 128, true) || !concreteResourceModel(route.Model) || !resourceText(route.Group, 64, true) || pairs[route.ResourcePublicationPair] {
			return false
		}
		caps, _ := json.Marshal(route.Capabilities)
		if resourceGatewayPolicyForRoute(ResourceGatewayRoute{Capabilities: string(caps), ContextLength: route.ContextLength, GroupDefinitionID: groupID}) == nil {
			return false
		}
		pairs[route.ResourcePublicationPair] = true
		used[route.GatewayChannelID] = true
	}
	return len(used) == len(targets)
}

func PublishResourceGateway(ctx context.Context, expected ResourcePublicationPrecondition, bindings []ResourceChannelBinding, reviewDigest string, actorID int64) (*ResourcePublicationAcknowledgement, error) {
	raw, err := hex.DecodeString(reviewDigest)
	if err != nil || len(raw) != 32 || len(reviewDigest) != 64 || actorID <= 0 || expected.ResourceRevision < 0 || expected.ResourceRevision >= maxResourceRevision || expected.ChannelRevision < 0 || expected.ChannelRevision >= maxResourceRevision || expected.OptionVersion < 0 || expected.OptionVersion > maxResourceRevision || len(bindings) == 0 || len(bindings) > 500 {
		return nil, ErrInvalidResource
	}
	var result *ResourcePublicationAcknowledgement
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Claim the writer without advancing either version until review revalidation.
		locked := tx.Model(&ResourceCatalogRecord{}).Where("id = 1 AND revision = ?", expected.ResourceRevision).UpdateColumn("revision", gorm.Expr("revision"))
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected != 1 {
			return ErrResourceConflict
		}
		preview, err := previewResourcePublicationInTransaction(tx, expected, bindings)
		if err != nil {
			return err
		}
		if preview.ReviewDigest != reviewDigest {
			return ErrResourceConflict
		}
		if !preview.Ready {
			return fmt.Errorf("%w: publication has blocking issues", ErrInvalidResource)
		}
		encodedBindings, err := json.Marshal(preview.Bindings)
		if err != nil {
			return err
		}
		encodedRoutes, err := json.Marshal(preview.ProposedRoutes)
		if err != nil {
			return err
		}
		receipt := ResourceGatewayPublication{ActorID: actorID, Operation: "publish", Profile: ResourceGatewayTextProfile, ReviewDigest: reviewDigest, ResourceRevision: expected.ResourceRevision + 1, ChannelRevision: expected.ChannelRevision + 1, OptionVersion: expected.OptionVersion, Bindings: string(encodedBindings), Routes: string(encodedRoutes)}
		if err := tx.Create(&receipt).Error; err != nil {
			return err
		}
		ids := make([]int64, 0, len(preview.Bindings))
		for _, binding := range preview.Bindings {
			id, _ := strconv.ParseInt(binding.GatewayChannelID, 10, 64)
			ids = append(ids, id)
			owner := ResourceGatewayManagedChannel{ChannelID: id, SourceChannelID: binding.SourceChannelID, Active: true, Profile: ResourceGatewayTextProfile, PublicationID: receipt.ID}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "channel_id"}}, DoUpdates: clause.AssignmentColumns([]string{"source_channel_id", "active", "profile", "publication_id"})}).Create(&owner).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("channel_id IN ?", ids).Delete(&ResourceGatewayRoute{}).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_id IN ?", ids).Delete(&Ability{}).Error; err != nil {
			return err
		}
		routes := make([]ResourceGatewayRoute, 0, len(preview.ProposedRoutes))
		for _, route := range preview.ProposedRoutes {
			id, _ := strconv.ParseInt(route.GatewayChannelID, 10, 64)
			groupID, _ := strconv.ParseInt(route.GroupDefinitionID, 10, 64)
			capabilities, _ := json.Marshal(route.Capabilities)
			routes = append(routes, ResourceGatewayRoute{ChannelID: id, Model: route.Model, Group: route.Group, Capabilities: string(capabilities), ContextLength: route.ContextLength, GroupDefinitionID: groupID})
		}
		if err := tx.CreateInBatches(routes, 100).Error; err != nil {
			return err
		}
		var channels []Channel
		if err := tx.Select("id", "type", "status", "priority", "weight").Where("id IN ?", ids).Find(&channels).Error; err != nil {
			return err
		}
		if len(channels) != len(ids) {
			return ErrResourceConflict
		}
		for i := range channels {
			if managed, err := addResourceGatewayAbilities(tx, &channels[i]); err != nil {
				return err
			} else if !managed {
				return ErrResourceConflict
			}
		}
		if err := advanceChannelConfigurationRevision(tx); err != nil {
			return err
		}
		if err := tx.Model(&ResourceCatalogRecord{}).Where("id = 1").Updates(map[string]any{"revision": expected.ResourceRevision + 1, "updated_at": time.Now().Unix()}).Error; err != nil {
			return err
		}
		result = &ResourcePublicationAcknowledgement{PublicationID: strconv.FormatInt(receipt.ID, 10), ReviewDigest: reviewDigest, ResourceRevision: receipt.ResourceRevision, ChannelRevision: receipt.ChannelRevision, OptionVersion: receipt.OptionVersion, Profile: ResourceGatewayTextProfile, BindingCount: len(preview.Bindings), RouteCount: len(preview.ProposedRoutes), Published: true, Operation: receipt.Operation}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
