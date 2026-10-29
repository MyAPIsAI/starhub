package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"starhub/common"
	"starhub/model"
	"strings"
)

func ListOwnedTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := model.ListTokens(requestUserID(r))
	if err != nil {
		common.ApiError(w, 500, "owned keys unavailable")
		return
	}
	data := make([]map[string]any, 0, len(tokens))
	for i := range tokens {
		data = append(data, maskedToken(&tokens[i]))
	}
	common.ApiSuccess(w, data)
}
func GetOwnedTokenChoices(w http.ResponseWriter, r *http.Request) {
	owner, ok := expectedAccountOwner(w, r)
	if !ok {
		return
	}
	state, err := model.ReadOwnedTokenChoices(owner)
	if err != nil {
		common.ApiError(w, 500, "key choices unavailable")
		return
	}
	coverage, err := model.ListModelCoverage()
	if err != nil {
		common.ApiError(w, 500, "model protocol choices unavailable")
		return
	}
	metadata, err := model.ListModelMeta()
	if err != nil {
		common.ApiError(w, 500, "model metadata unavailable")
		return
	}
	for i := range state.Choices {
		choice := &state.Choices[i]
		if item, ok := coverage[choice.Model]; ok {
			var tags []string
			if meta := metadata[choice.Model]; meta != nil {
				tags = model.SplitList(meta.Tags)
			}
			choice.Protocols = plazaProtocols(item.ChannelTypes[choice.Group], tags)
			if containsString(tags, "responses") {
				for _, kind := range item.ChannelTypes[choice.Group] {
					if kind == 1 {
						choice.Protocols = append(choice.Protocols, "Responses")
						break
					}
				}
			}
		}
	}
	common.ApiSuccess(w, map[string]any{"schema": "owned_token_choices_v1", "guarded_actions": true, "user_id": owner, "revision": state.Revision, "choices": state.Choices})
}

type ownedTokenAction struct {
	Schema          string             `json:"schema"`
	ExpectedUserID  int64              `json:"expected_user_id"`
	OperationID     string             `json:"operation_id"`
	Action          string             `json:"action"`
	TokenID         int64              `json:"token_id"`
	Revision        string             `json:"revision"`
	ChoicesRevision string             `json:"choices_revision"`
	Payload         tokenUpdateRequest `json:"payload"`
}

func readOwnedTokenAction(w http.ResponseWriter, r *http.Request) (*ownedTokenAction, bool) {
	defer r.Body.Close()
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	d.DisallowUnknownFields()
	var input ownedTokenAction
	if d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF || input.Schema != "owned_token_action_v1" || !canonicalDeactivationHex(input.OperationID, 32) || input.ExpectedUserID <= 0 {
		common.ApiError(w, 400, "invalid owned key action")
		return nil, false
	}
	if input.ExpectedUserID != requestUserID(r) {
		common.ApiError(w, 409, "current account changed")
		return nil, false
	}
	return &input, true
}
func WriteOwnedToken(w http.ResponseWriter, r *http.Request) {
	input, ok := readOwnedTokenAction(w, r)
	if !ok {
		return
	}
	if !canonicalDeactivationHex(input.ChoicesRevision, 64) || (input.Action != "create" && (!canonicalDeactivationHex(input.Revision, 64) || input.TokenID <= 0)) {
		common.ApiError(w, 400, "key snapshot required")
		return
	}
	p := input.Payload
	fields := map[string]any{}
	if p.Name != nil {
		fields["name"] = strings.TrimSpace(*p.Name)
	}
	if p.Group != nil {
		fields["group"] = strings.TrimSpace(*p.Group)
	}
	if p.ModelLimits != nil {
		fields["model_limits"] = strings.TrimSpace(*p.ModelLimits)
	}
	if p.AllowIps != nil {
		fields["allow_ips"] = strings.TrimSpace(*p.AllowIps)
	}
	if p.AutoGroups != nil {
		fields["auto_groups"] = strings.TrimSpace(*p.AutoGroups)
	}
	if p.RemainQuota != nil {
		fields["remain_quota"] = *p.RemainQuota
	}
	if p.UnlimitedQuota != nil {
		fields["unlimited_quota"] = *p.UnlimitedQuota
		if *p.UnlimitedQuota {
			fields["remain_quota"] = int64(0)
		}
	}
	if p.Status != nil {
		fields["status"] = *p.Status
	}
	if p.ExpiredTime != nil {
		fields["expired_time"] = *p.ExpiredTime
	}
	for _, v := range []string{stringValue(fields, "model_limits"), stringValue(fields, "allow_ips"), stringValue(fields, "auto_groups")} {
		if len(v) > 1024 {
			common.ApiError(w, 400, "key policy exceeds limit")
			return
		}
	}
	if err := model.ValidOwnedTokenFields(fields); err != nil {
		common.ApiError(w, 400, err.Error())
		return
	}
	switch input.Action {
	case "create":
		if _, ok := fields["name"]; !ok {
			common.ApiError(w, 400, "key name required")
			return
		}
	case "update":
		if len(fields) == 0 {
			common.ApiError(w, 400, "key fields required")
			return
		}
	case "delete", "regenerate":
		if len(fields) > 0 {
			common.ApiError(w, 400, "unexpected key fields")
			return
		}
	default:
		common.ApiError(w, 400, "unknown key action")
		return
	}
	if input.Action != "create" {
		token, err := model.GetTokenByID(input.TokenID)
		if err != nil || token.UserID != input.ExpectedUserID {
			common.ApiError(w, 409, "owned key changed")
			return
		}
		if p.ExpectedQuota != nil && *p.ExpectedQuota != token.RemainQuota {
			common.ApiError(w, 409, "key balance changed")
			return
		}
		if p.UnlimitedQuota != nil && !*p.UnlimitedQuota && token.UnlimitedQuota && p.RemainQuota == nil {
			common.ApiError(w, 400, "remaining quota required")
			return
		}
	}
	token, receipt, err := model.MutateOwnedToken(input.ExpectedUserID, input.OperationID, input.Action, input.TokenID, input.Revision, input.ChoicesRevision, fields)
	if err != nil {
		if errors.Is(err, model.ErrOwnedTokenConflict) {
			common.ApiError(w, 409, "key or configuration changed; reload and confirm again")
		} else {
			common.ApiError(w, 400, "key action was rejected; verify the selected group and models")
		}
		return
	}
	out := map[string]any{"schema": "owned_token_receipt_v1", "user_id": receipt.UserID, "operation_id": receipt.OperationID, "action": receipt.Action, "token_id": receipt.TokenID, "revision": receipt.Revision}
	if input.Action == "create" || input.Action == "regenerate" {
		public := maskedToken(token)
		public["key"] = token.Key
		out["token"] = public
	}
	common.ApiSuccess(w, out)
}
func stringValue(fields map[string]any, key string) string {
	value, _ := fields[key].(string)
	return value
}
func GetOwnedTokenReceipt(w http.ResponseWriter, r *http.Request) {
	input, ok := readOwnedTokenAction(w, r)
	if !ok {
		return
	}
	receipt, err := model.ReadOwnedTokenReceipt(input.ExpectedUserID, input.OperationID)
	if err != nil {
		common.ApiError(w, 404, "key receipt not found")
		return
	}
	common.ApiSuccess(w, map[string]any{"schema": "owned_token_receipt_v1", "user_id": receipt.UserID, "operation_id": receipt.OperationID, "action": receipt.Action, "token_id": receipt.TokenID, "revision": receipt.Revision})
}
