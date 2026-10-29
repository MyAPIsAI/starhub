package controller

import (
	"errors"
	"net/http"
	"strconv"

	"starhub/common"
	"starhub/model"
)

func GetDeactivationState(w http.ResponseWriter, r *http.Request) {
	owner, ok := expectedAccountOwner(w, r)
	if !ok {
		return
	}
	state, err := model.ReadAccountDeactivationState(owner)
	if err != nil {
		if errors.Is(err, model.ErrDeactivationUserUnavailable) {
			common.ApiError(w, http.StatusUnauthorized, "account unavailable")
			return
		}
		common.ApiError(w, http.StatusInternalServerError, "account deactivation preview is unavailable")
		return
	}
	common.ApiSuccess(w, map[string]any{
		"schema": "deactivation_state_v1", "policy": "dust_below_cent_v1", "guarded_deactivation": false,
		"user_id": state.UserID, "account_revision": state.AccountRevision,
		"wallet_balance_nano": strconv.FormatInt(state.Balance, 10), "pending_recharges": state.PendingRecharges,
		"last_active_root": state.LastActiveRoot, "requires_dust_acknowledgement": state.RequiresDustAcknowledgement,
		"dust_limit_nano": strconv.FormatInt(model.AccountDeactivationDustLimit, 10), "blocked_reasons": state.BlockedReasons,
	})
}
