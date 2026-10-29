package relay

import (
	"context"
	"errors"
	"starhub/common"
	"starhub/model"
	"time"
)

var ErrResponseAccess = errors.New("stored response is unavailable for this API key")

func OwnedResponseReference(ctx context.Context, id string) (*model.ResponseReference, *model.Channel, string, error) {
	user, _ := ctx.Value(common.CtxUserID).(int64)
	token, _ := ctx.Value(common.CtxTokenID).(int64)
	ref, err := model.LookupResponseReference(ctx, user, token, id, time.Now())
	if err != nil {
		return nil, nil, "", ErrResponseAccess
	}
	group, _ := ctx.Value(common.CtxUsingGroup).(string)
	allowed := group == ref.Group
	if group == common.AutoGroup {
		groups, _ := ctx.Value(common.CtxAutoGroups).([]string)
		for _, g := range groups {
			allowed = allowed || g == ref.Group
		}
	}
	limits, _ := ctx.Value(common.CtxTokenModelLimits).(string)
	if !allowed || !(&model.Token{ModelLimits: limits}).AllowsModel(ref.Model) {
		return nil, nil, "", ErrResponseAccess
	}
	definition, err := model.GetServiceGroup(ref.Group)
	if err != nil && !errors.Is(err, model.ErrServiceGroupNotFound) {
		return nil, nil, "", err
	}
	definitionID := int64(0)
	if definition != nil {
		definitionID = definition.ID
	}
	if definitionID != ref.GroupDefinitionID {
		return nil, nil, "", ErrResponseAccess
	}
	ch, err := model.GetChannelByID(ref.ChannelID)
	if err != nil {
		return nil, nil, "", ErrResponseAccess
	}
	key, ok := model.ResponseCredential(ch, ref.CredentialIdentity)
	if !ok {
		return nil, nil, "", ErrResponseAccess
	}
	var count int64
	if err := model.DB.WithContext(ctx).Model(&model.Ability{}).Where("channel_id = ? AND \"group\" = ? AND model = ? AND enabled = ?", ch.ID, ref.Group, ref.Model, true).Count(&count).Error; err != nil {
		return nil, nil, "", err
	}
	if count == 0 {
		return nil, nil, "", ErrResponseAccess
	}
	return ref, ch, key, nil
}
