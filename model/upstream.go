package model

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"time"

	"starhub/common"
	"starhub/internal/upstream"
	"starhub/setting"
)

type ProtectedChannelsError struct{ Channels []int64 }

func (e *ProtectedChannelsError) Error() string { return "no unprotected upstream channel" }
func (e *ProtectedChannelsError) Unwrap() error { return ErrNoChannel }

func UpstreamIdentity(ch *Channel, name, key string) upstream.Identity {
	if ch.RuntimeModel != "" {
		name = ch.RuntimeModel
	}
	return upstream.Identity{ChannelID: ch.ID, Model: name, Generation: UpstreamEndpointGeneration(ch), Credential: upstream.CredentialID(key)}
}

// Protection follows the endpoint and individual credential, not key-list order.
func UpstreamEndpointGeneration(ch *Channel) string {
	return upstream.CredentialID(fmt.Sprintf("%d|%d|%s", ch.ID, ch.Type, ch.BaseURL))
}

// A probe may not restore a manual disable or an external authority denial.
func UpstreamProbeAllowed(ch *Channel, actual string, paths ...string) (bool, error) {
	var abilities []Ability
	if err := DB.Where("channel_id = ? AND enabled = ?", ch.ID, true).Where(resourceGatewayAbilityCondition("abilities")).Find(&abilities).Error; err != nil {
		return false, err
	}
	for _, ability := range abilities {
		policy, managed, err := ReadResourceGatewayPolicy(context.Background(), ch.ID, ability.Group, ability.Model)
		if err != nil {
			return false, err
		}
		name := ability.Model
		if managed {
			if policy == nil {
				continue
			}
			if policy.UpstreamModel() != "" {
				name = policy.UpstreamModel()
			}
			if len(paths) > 0 && !policy.Allows(paths[0], nil, 1, 16) {
				continue
			}
		}
		if name == actual {
			if setting.IsCommercialMode() {
				group, err := GetRoutingGroupPolicy(ability.Group)
				if err != nil {
					return false, err
				}
				allowed := false
				routingSnapshotState.RLock()
				for _, item := range routingSnapshotState.items {
					if item.ChannelID == strconv.FormatInt(ch.ID, 10) && item.ModelName == ability.Model && routeItemAllowed(item, group, true) && (item.Protection == nil || item.Protection.EndpointGeneration == ChannelEndpointGeneration(ch)) {
						allowed = true
						break
					}
				}
				routingSnapshotState.RUnlock()
				if !allowed {
					continue
				}
			}
			return true, nil
		}
	}
	return false, nil
}

// PrepareUpstreamAdmission keeps aliases and credential restrictions consistent
// across weighted, sticky, hashed and scored selection.
func PrepareUpstreamAdmission(ch *Channel, group, name string, profiles ...upstream.Profile) (bool, error) {
	err := PrepareUpstreamAdmissions([]*Channel{ch}, group, name, profiles...)
	return len(ch.RuntimeKeys) > 0, err
}

func PrepareUpstreamAdmissions(channels []*Channel, group, name string, profiles ...upstream.Profile) error {
	profile := upstream.Profile{Capability: "chat"}
	if len(profiles) > 0 {
		profile = profiles[0]
	}
	all := []upstream.Identity{}
	byChannel := map[int64][]upstream.Identity{}
	for start := 0; start < len(channels); start += 200 {
		batch := channels[start:min(start+200, len(channels))]
		ids := make([]int64, 0, len(batch))
		for _, ch := range batch {
			ids = append(ids, ch.ID)
			ch.RuntimeKeys = map[string]bool{}
		}
		var owners []ResourceGatewayManagedChannel
		var routes []ResourceGatewayRoute
		err := DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("channel_id IN ?", ids).Find(&owners).Error; err != nil {
				return err
			}
			if len(owners) == 0 {
				return nil
			}
			return tx.Table("resource_gateway_routes AS rgr").Select("rgr.*").Joins("JOIN abilities a ON a.channel_id = rgr.channel_id AND a.model = rgr.model AND a.\"group\" = rgr.\"group\"").Where("rgr.channel_id IN ? AND rgr.model = ? AND rgr.\"group\" = ? AND a.enabled = TRUE", ids, name, group).Where(resourceGatewayAbilityCondition("a")).Find(&routes).Error
		})
		if err != nil {
			return err
		}
		managed := map[int64]bool{}
		policies := map[int64]*ResourceGatewayPolicy{}
		for _, owner := range owners {
			managed[owner.ChannelID] = true
		}
		for _, route := range routes {
			policies[route.ChannelID] = resourceGatewayPolicyForRoute(route)
		}
		for _, ch := range batch {
			ch.RuntimeModel = ""
			if managed[ch.ID] {
				if policies[ch.ID] == nil {
					continue
				}
				ch.RuntimeModel = policies[ch.ID].UpstreamModel()
			}
			keys := ch.Keys()
			if len(keys) == 0 {
				keys = []string{""}
			}
			statuses := ch.KeyStatuses()
			for index, key := range keys {
				if index < len(statuses) && statuses[index] != common.StatusEnabled {
					continue
				}
				identity := UpstreamIdentity(ch, name, key)
				identity.Capability, identity.Stream = profile.Capability, profile.Stream
				all = append(all, identity)
				byChannel[ch.ID] = append(byChannel[ch.ID], identity)
			}
		}
	}
	blocked, err := upstream.Blocked(DB, all, upstream.CurrentPolicy())
	if err != nil {
		return err
	}
	for _, ch := range channels {
		for _, identity := range byChannel[ch.ID] {
			if !upstream.IsBlocked(identity, blocked) {
				ch.RuntimeKeys[identity.Credential] = true
			}
		}
	}
	return nil
}

func UpstreamStateCurrent(ch *Channel, s upstream.State) bool {
	if UpstreamEndpointGeneration(ch) != s.Generation {
		return false
	}
	if s.Scope == "model" || s.Scope == "interface" {
		return true
	}
	keys := ch.Keys()
	if len(keys) == 0 {
		keys = []string{""}
	}
	for _, key := range keys {
		if upstream.CredentialID(key) == s.Credential {
			return true
		}
	}
	return false
}

// Interface/model circuits may use another enabled credential after key removal.
// Credential-scoped isolation always retains the exact failing credential.
func PrepareUpstreamProbe(ctx context.Context, id string) error {
	db := DB.WithContext(ctx)
	var s upstream.State
	if err := db.First(&s, "id = ?", id).Error; err != nil {
		return err
	}
	if s.Scope != "interface" && s.Scope != "model" {
		return nil
	}
	if s.LeaseUntil > time.Now().Unix() {
		return upstream.ErrBlocked
	}
	ch, err := GetChannelByID(s.ChannelID)
	if err != nil {
		return err
	}
	if !UpstreamStateCurrent(ch, s) || ch.Status != common.ChannelStatusEnabled {
		return upstream.ErrBlocked
	}
	keys := ch.Keys()
	if len(keys) == 0 {
		keys = []string{""}
	}
	statuses := ch.KeyStatuses()
	identities := []upstream.Identity{}
	for index, key := range keys {
		if index < len(statuses) && statuses[index] != common.StatusEnabled {
			continue
		}
		identity := s.Identity
		identity.Credential = upstream.CredentialID(key)
		if identity.Credential == s.Credential {
			return nil
		}
		identities = append(identities, identity)
	}
	blocked, err := upstream.Blocked(db, identities, upstream.CurrentPolicy())
	if err != nil {
		return err
	}
	delete(blocked, s.ID)
	credential := ""
	for _, identity := range identities {
		if upstream.IsBlocked(identity, blocked) {
			continue
		}
		if credential == "" || identity.Credential == s.Credential {
			credential = identity.Credential
		}
	}
	if credential == "" {
		if err := db.Model(&upstream.State{}).Where("id = ? AND version = ?", id, s.Version).Update("updated", time.Now().Unix()).Error; err != nil {
			return err
		}
		return upstream.ErrBlocked
	}
	if credential == s.Credential {
		return nil
	}
	result := db.Model(&upstream.State{}).Where("id = ? AND version = ? AND lease_until <= ?", id, s.Version, time.Now().Unix()).Updates(map[string]any{"credential": credential, "version": gorm.Expr("version + 1")})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return upstream.ErrBlocked
	}
	return nil
}

// Scan through obsolete rows before applying the visible/due result limit.
func CurrentUpstreamStates(ctx context.Context, due bool, limit int) ([]upstream.State, error) {
	result := []upstream.State{}
	q := DB.WithContext(ctx).Model(&upstream.State{})
	now := time.Now().Unix()
	if due {
		q = q.Where("open_until > 0 AND open_until <= ? AND lease_until <= ?", now, now).Order("updated ASC, id ASC")
	} else {
		q = q.Where("open_until <> 0 OR updated > ?", now-86400).Order("open_until DESC, updated DESC, id ASC")
	}
	for offset := 0; ; offset += 200 {
		var rows []upstream.State
		if err := q.Limit(200).Offset(offset).Find(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		ids := []int64{}
		for _, s := range rows {
			ids = append(ids, s.ChannelID)
		}
		var channels []Channel
		if err := DB.WithContext(ctx).Where("id IN ?", ids).Find(&channels).Error; err != nil {
			return nil, err
		}
		current := map[int64]*Channel{}
		for i := range channels {
			current[channels[i].ID] = &channels[i]
		}
		for _, s := range rows {
			ch := current[s.ChannelID]
			if ch != nil && UpstreamStateCurrent(ch, s) && (!due || ch.Status == common.ChannelStatusEnabled) {
				result = append(result, s)
				if len(result) >= limit {
					return result, nil
				}
			}
		}
		if len(rows) < 200 {
			break
		}
	}
	return result, nil
}

func CleanupUpstreamStates(ctx context.Context) error {
	var rows []upstream.State
	return DB.WithContext(ctx).Order("id").FindInBatches(&rows, 200, func(tx *gorm.DB, _ int) error {
		ids := []int64{}
		for _, s := range rows {
			ids = append(ids, s.ChannelID)
		}
		var channels []Channel
		if err := DB.WithContext(ctx).Where("id IN ?", ids).Find(&channels).Error; err != nil {
			return err
		}
		current := map[int64]*Channel{}
		for i := range channels {
			current[channels[i].ID] = &channels[i]
		}
		for _, s := range rows {
			ch := current[s.ChannelID]
			if s.LeaseUntil > time.Now().Unix() {
				continue
			}
			if ch == nil || !UpstreamStateCurrent(ch, s) || s.OpenUntil == 0 && s.Updated < time.Now().Add(-7*24*time.Hour).Unix() {
				if err := DB.WithContext(ctx).Where("id = ? AND version = ? AND lease_until <= ?", s.ID, s.Version, time.Now().Unix()).Delete(&upstream.State{}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	}).Error
}

func (ch *Channel) ProtectedKey(preferred string, index int) (string, int) {
	if ch.RuntimeKeys == nil || ch.RuntimeKeys[upstream.CredentialID(preferred)] {
		return preferred, index
	}
	copy := *ch
	statuses := ch.KeyStatuses()
	for i, key := range ch.Keys() {
		if !ch.RuntimeKeys[upstream.CredentialID(key)] {
			statuses[i] = common.StatusDisabled
		}
	}
	parts := make([]string, len(statuses))
	for i, value := range statuses {
		parts[i] = strconv.Itoa(value)
	}
	copy.MultiKeyStatus = strings.Join(parts, ",")
	return copy.SelectKey()
}
