package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"starhub/common"
	"starhub/middleware"
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
		"schema": "deactivation_state_v1", "policy": "dust_below_cent_v1", "guarded_deactivation": true,
		"user_id": state.UserID, "account_revision": state.AccountRevision,
		"wallet_balance_nano": strconv.FormatInt(state.Balance, 10), "pending_recharges": state.PendingRecharges,
		"last_active_root": state.LastActiveRoot, "requires_dust_acknowledgement": state.RequiresDustAcknowledgement,
		"dust_limit_nano": strconv.FormatInt(model.AccountDeactivationDustLimit, 10), "blocked_reasons": state.BlockedReasons,
	})
}

type deactivationInput struct {
	Schema          string `json:"schema"`
	ExpectedUserID  int64  `json:"expected_user_id"`
	AccountRevision string `json:"account_revision"`
	OperationID     string `json:"operation_id"`
	AcknowledgeDust bool   `json:"acknowledge_dust"`
}

func readDeactivationInput(w http.ResponseWriter, r *http.Request) (*deactivationInput, bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	var input deactivationInput
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || input.Schema != "deactivation_action_v1" || input.ExpectedUserID <= 0 || !canonicalDeactivationHex(input.OperationID, 32) || !canonicalDeactivationHex(input.AccountRevision, 64) {
		common.ApiError(w, 400, "invalid account deactivation request")
		return nil, false
	}
	return &input, true
}

func canonicalDeactivationHex(value string, size int) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(value) == size && hex.EncodeToString(decoded) == value
}

func deactivationSessionDigest(r *http.Request) string {
	digest := sha256.Sum256([]byte(middleware.SessionToken(r)))
	return hex.EncodeToString(digest[:])
}

func writeDeactivationReceipt(w http.ResponseWriter, receipt *model.AccountDeactivationReceipt) {
	common.ApiSuccess(w, map[string]any{"schema": "deactivation_receipt_v1", "user_id": receipt.UserID, "operation_id": receipt.OperationID, "account_revision": receipt.AccountRevision, "written_off_nano": receipt.WrittenOffNano, "closed": true, "keys_disabled": true})
}

func deactivateOwnedAccount(w http.ResponseWriter, r *http.Request) {
	input, ok := readDeactivationInput(w, r)
	if !ok {
		return
	}
	if input.ExpectedUserID != requestUserID(r) {
		common.ApiError(w, 409, "session identity changed; confirm the current account")
		return
	}
	receipt, err := model.DeactivateOwnedAccount(input.ExpectedUserID, input.AccountRevision, input.OperationID, deactivationSessionDigest(r), input.AcknowledgeDust)
	if err != nil {
		if errors.Is(err, model.ErrDeactivationConflict) || errors.Is(err, model.ErrDeactivationUserUnavailable) {
			common.ApiError(w, 409, "account changed or closure is blocked; reload the preview")
		} else {
			common.ApiError(w, 500, "account closure could not be confirmed")
		}
		return
	}
	middleware.Sessions.RemoveAllExcept(input.ExpectedUserID, "")
	writeDeactivationReceipt(w, receipt)
}

// GetDeactivationReceipt is a read-only recovery path bound to the exact old
// session credential and random operation ID. It cannot authorize a new close.
func GetDeactivationReceipt(w http.ResponseWriter, r *http.Request) {
	input, ok := readDeactivationInput(w, r)
	if !ok {
		return
	}
	if middleware.SessionToken(r) == "" {
		common.ApiError(w, 404, "closure receipt not found")
		return
	}
	receipt, err := model.ReadDeactivationReceipt(input.ExpectedUserID, input.OperationID, deactivationSessionDigest(r))
	if err != nil || receipt.AccountRevision != input.AccountRevision {
		common.ApiError(w, 404, "closure receipt not found")
		return
	}
	writeDeactivationReceipt(w, receipt)
}
