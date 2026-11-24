package model

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"starhub/common"
	"starhub/internal/upstream"
)

type RoutingStage uint8

const (
	RoutingNormal RoutingStage = iota
	RoutingPlatformFallback
	RoutingOrdinaryFallback
)

func (s RoutingStage) String() string {
	switch s {
	case RoutingPlatformFallback:
		return "platform_fallback"
	case RoutingOrdinaryFallback:
		return "ordinary_fallback"
	default:
		return "normal"
	}
}

// Only the internal final-stage selector can issue this request-local permit.
type fallbackPermit struct {
	group, model, generation string
}

func routeFallbackOnly(item RouteItem) bool {
	return item.ScoreEvidence != nil && item.ScoreEvidence.Lifecycle != nil && item.ScoreEvidence.Lifecycle.Admission == "fallback_only"
}

// Commercial admission can be bypassed, but manual and catalog denials cannot.
// Unknown disabled reasons remain closed until their provenance is known.
func ordinaryPlatformAdmission(ch *Channel, name string) (bool, string) {
	routingSnapshotState.RLock()
	defer routingSnapshotState.RUnlock()
	if routingSnapshotState.snapshot == nil {
		return true, "snapshot_unavailable"
	}
	overrides := []string{"platform_admission"}
	for _, item := range routingSnapshotState.items {
		if item.ChannelID != strconv.FormatInt(ch.ID, 10) || item.ModelName != name {
			continue
		}
		p := item.Protection
		if p == nil {
			if item.State == "disabled" {
				return false, ""
			}
			continue
		}
		if p.CatalogExcluded || p.Mode == "manual" || p.Reason == "operator_disabled" || p.Reason == "operator_confirmed" {
			return false, ""
		}
		switch p.Mode {
		case "allowed", "review":
		case "recovering":
			if p.Reason != "endpoint_changed" {
				return false, ""
			}
		case "disabled":
			if p.Reason != "half_open_failures" && p.Reason != "recovery_failed" {
				return false, ""
			}
		default:
			return false, ""
		}
		if p.EndpointGeneration != ChannelEndpointGeneration(ch) {
			overrides = append(overrides, "stale_endpoint_health")
			continue
		}
		overrides = append(overrides, fmt.Sprintf("platform:%s:v%d:%s:%s", RoutingResourceKey(item), p.StateVersion, p.Mode, p.Reason))
	}
	return true, strings.Join(overrides, "; ")
}

// CaptureChannelAttempt rechecks current authorization and the exact credential
// before allowing a final fallback to cross historical automatic protection.
func CaptureChannelAttempt(ctx context.Context, ch *Channel, group, name, key string, profile upstream.Profile, policy upstream.Policy, checks ...func(*Channel, string) (bool, error)) (*upstream.Attempt, error) {
	identity := UpstreamIdentity(ch, name, key)
	identity.Capability, identity.Stream = profile.Capability, profile.Stream
	var attempt *upstream.Attempt
	var err error
	if ch.RuntimeStage != RoutingOrdinaryFallback {
		attempt, err = upstream.Capture(DB.WithContext(ctx), identity, policy)
	} else {
		permit := ch.runtimeFallback
		if permit == nil || permit.group != group || permit.model != name || permit.generation != ChannelEndpointGeneration(ch) {
			return nil, upstream.ErrBlocked
		}
		var current Channel
		if err := DB.WithContext(ctx).First(&current, ch.ID).Error; err != nil {
			return nil, err
		}
		if current.Status != common.ChannelStatusEnabled || ChannelEndpointGeneration(&current) != permit.generation {
			return nil, upstream.ErrBlocked
		}
		var count int64
		if err := DB.WithContext(ctx).Model(&Ability{}).Where("channel_id = ? AND \"group\" = ? AND model = ? AND enabled = ?", ch.ID, group, name, true).Where(resourceGatewayAbilityCondition("abilities")).Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, upstream.ErrBlocked
		}
		if err := prepareUpstreamAdmissions([]*Channel{&current}, group, name, true, profile); err != nil {
			return nil, err
		}
		for _, check := range checks {
			if check == nil {
				continue
			}
			allowed, err := check(&current, group)
			if err != nil {
				return nil, err
			}
			if !allowed {
				return nil, upstream.ErrBlocked
			}
		}
		if current.RuntimeModel != ch.RuntimeModel || !current.RuntimeKeys[identity.Credential] {
			return nil, upstream.ErrBlocked
		}
		allowed, overrides := ordinaryPlatformAdmission(&current, name)
		if !allowed {
			return nil, upstream.ErrBlocked
		}
		routingSnapshotState.RLock()
		items := append([]RouteItem(nil), routingSnapshotState.items...)
		routingSnapshotState.RUnlock()
		var route *RoutingAttempt
		for _, item := range items {
			if item.ChannelID != strconv.FormatInt(ch.ID, 10) || item.ModelName != name || item.Protection == nil || item.Protection.EndpointGeneration != ChannelEndpointGeneration(&current) {
				continue
			}
			state, err := readLocalRoutingState(DB.WithContext(ctx), item)
			if err != nil {
				return nil, err
			}
			if state.OpenUntil > 0 {
				overrides += fmt.Sprintf("; local:%s:v%d:%s", state.ResourceKey, state.FailureGeneration, state.LastCause)
			}
			route = &RoutingAttempt{Item: item, FailureGeneration: state.FailureGeneration, OrdinaryFallback: true}
			break
		}
		attempt, err = upstream.CaptureFallback(DB.WithContext(ctx), identity, policy)
		if err == nil {
			ch.RuntimeRoute = route
			BeginRoutingAttempt(route)
			attempt.FallbackReason = "normal_and_platform_candidates_exhausted"
			attempt.ProtectionOverrides = strings.TrimSuffix(overrides+"; "+attempt.ProtectionOverrides, "; ")
		}
	}
	if err == nil {
		attempt.RoutingStage = ch.RuntimeStage.String()
		if ch.RuntimeStage == RoutingPlatformFallback {
			attempt.FallbackReason = "normal_candidates_exhausted"
		}
	}
	return attempt, err
}

// RecoverFallbackRouting closes only the captured local generation. The outbox
// and local recovery commit together; delivery is retried by the existing worker.
func RecoverFallbackRouting(attempt *RoutingAttempt, protection *upstream.Attempt, requestID string, now time.Time) error {
	if attempt == nil || !attempt.OrdinaryFallback || !attempt.Sent {
		return nil
	}
	return withLocalRoutingWrite(&attempt.Item, func(tx *gorm.DB) error {
		current, err := upstream.FallbackRecoveryCurrent(tx, protection)
		if err != nil || !current {
			return err
		}
		p := attempt.Item.Protection
		if p == nil {
			return nil
		}
		for _, current := range routingSnapshotState.items {
			if current.ResourceID == attempt.Item.ResourceID && (current.Protection == nil || current.Protection.StateVersion != p.StateVersion) {
				return nil
			}
		}
		state, err := readLocalRoutingState(tx, attempt.Item)
		if err != nil || state.FailureGeneration != attempt.FailureGeneration {
			return err
		}
		if state.OpenUntil == 0 && state.ConsecutiveFailures == 0 && state.HalfOpenFailures == 0 &&
			(state.LastCause == "fallback_recovered" || p.Mode == "allowed") {
			return nil
		}
		state.FailureGeneration++
		state.OpenUntil, state.BackoffSeconds, state.ConsecutiveFailures, state.HalfOpenFailures = 0, 0, 0, 0
		state.ProbeID, state.ProbeLeaseUntil, state.LastCause = "", 0, "fallback_recovered"
		if err := tx.Save(&state).Error; err != nil {
			return err
		}
		event := upstream.Event{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(attempt.AttemptID+":local_recovery")).String(), RequestID: requestID, ChannelID: protection.ChannelID, Model: protection.Model, ResourceID: protection.Identity.StateID("interface"), Kind: "recovery", Class: "fallback_recovered", Decision: "local_protection_cleared", Created: now.Unix(), Started: protection.Started, RoutingTrace: protection.RoutingTrace}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		return enqueueProtectionEvent(tx, attempt.Item, state, "fallback_success", attempt.AttemptID, now)
	})
}
