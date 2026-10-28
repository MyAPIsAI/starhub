package model

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"starhub/common"
)

const ResourceGatewayTextProfile = "openai_text_exact_v1"

// ResourceGatewayPolicy is immutable and only applies to a managed exact pair.
type ResourceGatewayPolicy struct {
	contextLength int64
	code          bool
}

func resourceGatewayPolicyForRoute(route ResourceGatewayRoute) *ResourceGatewayPolicy {
	if route.ContextLength < 0 || route.ContextLength > maxResourceRevision {
		return nil
	}
	switch route.Capabilities {
	case `["text"]`:
		return &ResourceGatewayPolicy{contextLength: route.ContextLength}
	case `["code"]`, `["code","text"]`:
		return &ResourceGatewayPolicy{contextLength: route.ContextLength, code: true}
	default:
		return nil
	}
}

func (policy *ResourceGatewayPolicy) Allows(path string, capabilities []string, input, output int64) bool {
	if policy == nil || (path != "/v1/chat/completions" && path != "/v1/messages") || input < 0 || output < 0 || input > maxResourceRevision || output > maxResourceRevision {
		return false
	}
	if policy.contextLength > 0 && (input > policy.contextLength || output > policy.contextLength-input) {
		return false
	}
	for _, capability := range capabilities {
		if capability != "text" && (capability != "code" || !policy.code) {
			return false
		}
	}
	return true
}

// Unknown or inactive managed policies deny access; missing unmanaged ownership
// leaves legacy channels unchanged. All policy reads share one database snapshot.
func ReadResourceGatewayPolicy(ctx context.Context, channelID int64, group, model string) (*ResourceGatewayPolicy, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var policy *ResourceGatewayPolicy
	managed := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var owner ResourceGatewayManagedChannel
		err := tx.First(&owner, "channel_id = ?", channelID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		managed = true
		if !owner.Active || owner.Profile != ResourceGatewayTextProfile {
			return nil
		}
		var ch Channel
		if err := tx.Select("id", "type", "status").First(&ch, "id = ?", channelID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if ch.Type != common.DefaultChannelType || ch.Status != common.ChannelStatusEnabled {
			return nil
		}
		var route ResourceGatewayRoute
		if err := tx.Where("channel_id = ? AND model = ? AND \"group\" = ?", channelID, model, group).First(&route).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		policy = resourceGatewayPolicyForRoute(route)
		return nil
	})
	return policy, managed, err
}
