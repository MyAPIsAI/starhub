package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"starhub/common"
)

const AccountDeactivationDustLimit int64 = 10_000_000

var ErrDeactivationUserUnavailable = errors.New("account unavailable")
var ErrDeactivationConflict = errors.New("account deactivation state changed or is blocked")

type AccountDeactivationReceipt struct {
	ID              int64  `gorm:"primaryKey" json:"-"`
	UserID          int64  `gorm:"uniqueIndex:idx_deactivation_operation;not null" json:"user_id"`
	OperationID     string `gorm:"uniqueIndex:idx_deactivation_operation;size:64;not null" json:"operation_id"`
	SessionDigest   string `gorm:"size:64;not null" json:"-"`
	AccountRevision string `gorm:"size:64;not null" json:"account_revision"`
	WrittenOffNano  string `gorm:"size:32;not null" json:"written_off_nano"`
	CreatedAt       int64  `gorm:"autoCreateTime" json:"created_at"`
}

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
		ID                                                                    int64
		Username, Email, Password, Group                                      string
		Role, Status                                                          int
		CreatedAt, Balance, PendingRecharges, OtherRoots, DeactivationVersion int64
	}{user.ID, user.Username, user.Email, user.Password, user.Group, user.Role, user.Status, user.CreatedAt, state.Balance, state.PendingRecharges, otherRoots, user.DeactivationVersion})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(raw)
	state.AccountRevision = hex.EncodeToString(digest[:])
	return state, nil
}

// DeactivateOwnedAccount obtains the SQLite writer lock before reading blockers.
// Recharge admission takes the same lock, so it cannot create a pending order
// between the final snapshot and the account transition.
func DeactivateOwnedAccount(owner int64, revision, operation, sessionDigest string, acknowledgeDust bool) (*AccountDeactivationReceipt, error) {
	var receipt AccountDeactivationReceipt
	err := DB.Transaction(func(tx *gorm.DB) error {
		lock := tx.Model(&User{}).Where("id = ? AND status = ?", owner, common.StatusEnabled).UpdateColumn("id", gorm.Expr("id"))
		if lock.Error != nil {
			return lock.Error
		}
		if lock.RowsAffected != 1 {
			return ErrDeactivationUserUnavailable
		}
		var used int64
		if err := tx.Model(&AccountDeactivationReceipt{}).Where("user_id = ? AND operation_id = ?", owner, operation).Count(&used).Error; err != nil {
			return err
		}
		if used != 0 {
			return ErrDeactivationConflict
		}
		state, err := readAccountDeactivationState(tx, owner)
		if err != nil {
			return err
		}
		if state.AccountRevision != revision || len(state.BlockedReasons) != 0 || (state.RequiresDustAcknowledgement && !acknowledgeDust) {
			return ErrDeactivationConflict
		}
		if state.Balance > 0 {
			updated := tx.Model(&Wallet{}).Where("user_id = ? AND balance = ?", owner, state.Balance).Updates(map[string]any{"balance": 0, "updated_at": time.Now().Unix()})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return ErrDeactivationConflict
			}
			entry := &LedgerEntry{UserID: owner, Type: "adjustment", Amount: -state.Balance, BalanceAfter: 0, Description: "Account closure dust write-off", TraceID: operation, FundingSource: FundingSourceLegacyUnverified}
			if err := tx.Create(entry).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&Token{}).Where("user_id = ?", owner).Update("status", common.StatusDisabled).Error; err != nil {
			return err
		}
		if err := tx.Model(&User{}).Where("id = ?", owner).Updates(map[string]any{"status": common.StatusDisabled, "deactivation_version": gorm.Expr("deactivation_version + 1")}).Error; err != nil {
			return err
		}
		receipt = AccountDeactivationReceipt{UserID: owner, OperationID: operation, SessionDigest: sessionDigest, AccountRevision: revision, WrittenOffNano: fmt.Sprint(state.Balance)}
		return tx.Create(&receipt).Error
	})
	if err != nil {
		return nil, err
	}
	return &receipt, nil
}

// ReadDeactivationReceipt requires both the random operation ID and the exact
// originating session credential, even after that credential was revoked.
func ReadDeactivationReceipt(owner int64, operation, sessionDigest string) (*AccountDeactivationReceipt, error) {
	var receipt AccountDeactivationReceipt
	err := DB.Where("user_id = ? AND operation_id = ? AND session_digest = ?", owner, operation, sessionDigest).First(&receipt).Error
	return &receipt, err
}
