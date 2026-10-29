package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"starhub/common"
)

const AccountDeactivationDustLimit int64 = 10_000_000

var ErrDeactivationUserUnavailable = errors.New("account unavailable")

type AccountDeactivationState struct {
	UserID                      int64
	AccountRevision             string
	Balance                     int64
	PendingRecharges            int64
	LastActiveRoot              bool
	RequiresDustAcknowledgement bool
	BlockedReasons              []string
}

// ReadAccountDeactivationState reads a consistent database snapshot without
// creating wallets, expiring orders, changing funds or granting a close action.
func ReadAccountDeactivationState(owner int64) (*AccountDeactivationState, error) {
	var state *AccountDeactivationState
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		state, err = readAccountDeactivationState(tx, owner)
		return err
	})
	return state, err
}

func readAccountDeactivationState(tx *gorm.DB, owner int64) (*AccountDeactivationState, error) {
	var user User
	if owner <= 0 {
		return nil, ErrDeactivationUserUnavailable
	}
	if err := tx.First(&user, "id = ?", owner).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDeactivationUserUnavailable
		}
		return nil, err
	}
	if user.Status != common.StatusEnabled {
		return nil, ErrDeactivationUserUnavailable
	}
	state := &AccountDeactivationState{UserID: owner, BlockedReasons: []string{}}
	var wallet Wallet
	if err := tx.Where("user_id = ?", owner).First(&wallet).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	state.Balance = wallet.Balance
	if err := tx.Model(&RechargeOrder{}).Where("user_id = ? AND status IN ?", owner, []string{RechargePending, RechargeProcessing}).Count(&state.PendingRecharges).Error; err != nil {
		return nil, err
	}
	var otherRoots int64
	if user.Role == common.RoleRoot {
		if err := tx.Model(&User{}).Where("id <> ? AND role = ? AND status = ?", owner, common.RoleRoot, common.StatusEnabled).Count(&otherRoots).Error; err != nil {
			return nil, err
		}
		state.LastActiveRoot = otherRoots == 0
	}
	state.RequiresDustAcknowledgement = state.Balance > 0 && state.Balance < AccountDeactivationDustLimit
	if state.Balance < 0 {
		state.BlockedReasons = append(state.BlockedReasons, "outstanding_balance")
	}
	if state.Balance >= AccountDeactivationDustLimit {
		state.BlockedReasons = append(state.BlockedReasons, "balance_requires_resolution")
	}
	if state.PendingRecharges > 0 {
		state.BlockedReasons = append(state.BlockedReasons, "pending_recharges")
	}
	if state.LastActiveRoot {
		state.BlockedReasons = append(state.BlockedReasons, "last_active_root")
	}
	// Hash the security snapshot; raw password hashes and contact data never
	// leave this function through the public preview contract.
	raw, err := json.Marshal(struct {
		ID                                               int64
		Username, Email, Password, Group                 string
		Role, Status                                     int
		CreatedAt, Balance, PendingRecharges, OtherRoots int64
	}{user.ID, user.Username, user.Email, user.Password, user.Group, user.Role, user.Status, user.CreatedAt, state.Balance, state.PendingRecharges, otherRoots})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(raw)
	state.AccountRevision = hex.EncodeToString(digest[:])
	return state, nil
}
