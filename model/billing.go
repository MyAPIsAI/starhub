package model

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"starhub/common"
)

var ErrRechargeOrderNotFound = errors.New("recharge order not found")

// Wallet stores a user's prepaid balance in nano-USD (1 USD = 1e9 nano).
// One row per user, created lazily on first recharge.
type Wallet struct {
	UserID    int64 `gorm:"primaryKey" json:"user_id"`
	Balance   int64 `gorm:"not null;default:0" json:"balance"` // nano-USD
	CreatedAt int64 `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt int64 `gorm:"autoUpdateTime" json:"updated_at"`
}

// LedgerEntry records every balance-changing event (recharge, consumption,
// adjustment, refund). amount is signed nano-USD; balance_after is the
// resulting wallet balance for audit reconstruction.
type LedgerEntry struct {
	ID            int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID        int64  `gorm:"index;not null" json:"user_id"`
	Type          string `gorm:"size:32;not null;default:''" json:"type"` // recharge | call_summary | adjustment | refund
	Amount        int64  `gorm:"not null;default:0" json:"amount"`        // signed nano-USD
	BalanceAfter  int64  `gorm:"not null;default:0" json:"balance_after"` // nano-USD
	Description   string `gorm:"size:256;not null;default:''" json:"description"`
	OrderID       string `gorm:"size:64;not null;default:''" json:"order_id"`
	TraceID       string `gorm:"size:64;not null;default:''" json:"trace_id"`
	FundingSource string `gorm:"size:32;not null;default:'legacy_unverified'" json:"funding_source"`
	CreatedAt     int64  `gorm:"autoCreateTime;index" json:"created_at"`
}

// RechargeOrder tracks a single top-up attempt. The state machine is:
// pending -> processing -> paid | failed | cancelled | expired
type RechargeOrder struct {
	ID            int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	OrderNumber   string `gorm:"uniqueIndex;size:64;not null" json:"order_number"` // human-readable "ord_..."
	UserID        int64  `gorm:"index;not null" json:"user_id"`
	Amount        int64  `gorm:"not null;default:0" json:"amount"`                 // nano-USD
	Status        string `gorm:"size:32;not null;default:'pending'" json:"status"` // pending | processing | paid | failed | cancelled | expired
	Channel       string `gorm:"size:32;not null;default:'card'" json:"channel"`
	FailureReason string `gorm:"size:64;not null;default:''" json:"failure_reason"`
	LatePayment   bool   `gorm:"not null;default:false" json:"late_payment"`
	CreatedAt     int64  `gorm:"autoCreateTime" json:"created_at"`
	ExpiresAt     int64  `gorm:"not null;default:0" json:"expires_at"` // unix seconds
	PaidAt        int64  `gorm:"not null;default:0" json:"paid_at"`    // unix seconds
	FundingSource string `gorm:"size:32;not null;default:'legacy_unverified'" json:"funding_source"`
}

const (
	FundingSourceLegacyUnverified = "legacy_unverified"
	FundingSourceSimulation       = "simulation"
)

// Existing or unrecognized provenance never becomes verified funding.
func PublicFundingSource(source string) string {
	if source == FundingSourceSimulation {
		return FundingSourceSimulation
	}
	return FundingSourceLegacyUnverified
}

// RechargeOrderExpirySeconds is how long a pending order stays payable.
const RechargeOrderExpirySeconds = 30 * 60

// RechargeStatus constants mirror the frontend RechargeStatus union.
const (
	RechargePending    = "pending"
	RechargeProcessing = "processing"
	RechargePaid       = "paid"
	RechargeFailed     = "failed"
	RechargeCancelled  = "cancelled"
	RechargeExpired    = "expired"
)

// --- Wallet ---

// GetOrCreateWallet returns the user's wallet, creating it with a zero
// balance on first access.
func GetOrCreateWallet(userID int64) (*Wallet, error) {
	var w Wallet
	err := DB.Where("user_id = ?", userID).First(&w).Error
	if err == nil {
		return &w, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	w = Wallet{UserID: userID, Balance: 0}
	if err := DB.Create(&w).Error; err != nil {
		// Another concurrent request may have created it; retry the read.
		err2 := DB.Where("user_id = ?", userID).First(&w).Error
		if err2 == nil {
			return &w, nil
		}
		return nil, fmt.Errorf("create wallet: %w", err)
	}
	return &w, nil
}

// --- Ledger ---

// ListLedger returns ledger entries for a user (or all for admin when
// userID == 0), newest first.
func ListLedger(userID int64) ([]LedgerEntry, error) {
	var entries []LedgerEntry
	q := DB.Model(&LedgerEntry{})
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	err := q.Order("id DESC").Find(&entries).Error
	return entries, err
}

// ListLedgerSince returns ledger entries since the given unix timestamp
// for a user (or all users when userID == 0), ordered newest first.
func ListLedgerSince(userID int64, since int64) ([]LedgerEntry, error) {
	var entries []LedgerEntry
	q := DB.Model(&LedgerEntry{}).Where("created_at >= ?", since)
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	err := q.Order("id DESC").Limit(10000).Find(&entries).Error
	return entries, err
}

// --- Recharge order ---

// CreateRechargeOrder inserts a new pending order with a 30-minute expiry.
func CreateRechargeOrder(o *RechargeOrder) error {
	// This entry point creates simulated orders only. Verified payments need
	// a separate authenticated provider workflow and must not reuse resolve.
	o.FundingSource = FundingSourceSimulation
	if o.OrderNumber == "" {
		o.OrderNumber = GenerateRechargeOrderNumber()
	}
	if o.Status == "" {
		o.Status = RechargePending
	}
	if o.ExpiresAt == 0 {
		o.ExpiresAt = time.Now().Unix() + RechargeOrderExpirySeconds
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		owner := tx.Model(&User{}).Where("id = ? AND status = ?", o.UserID, common.StatusEnabled).UpdateColumn("id", gorm.Expr("id"))
		if owner.Error != nil {
			return owner.Error
		}
		if owner.RowsAffected != 1 {
			return ErrDeactivationUserUnavailable
		}
		return tx.Create(o).Error
	})
}

// GetRechargeOrder returns an order by its numeric ID.
func GetRechargeOrder(id int64) (*RechargeOrder, error) {
	if id <= 0 {
		return nil, ErrRechargeOrderNotFound
	}
	var o RechargeOrder
	err := DB.First(&o, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRechargeOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// ListRechargeOrders returns orders for a user (or all for admin),
// newest first.
func ListRechargeOrders(userID int64) ([]RechargeOrder, error) {
	var orders []RechargeOrder
	q := DB.Model(&RechargeOrder{})
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	err := q.Order("id DESC").Find(&orders).Error
	return orders, err
}

// ResolveRechargeOrder atomically transitions an order to its terminal
// state and, on success, credits the wallet and writes a ledger entry.
// It returns the updated order. The transition is guarded so a
// concurrent double-resolve cannot double-credit.
func ResolveRechargeOrder(id int64, outcome string) (*RechargeOrder, error) {
	var result *RechargeOrder
	err := DB.Transaction(func(tx *gorm.DB) error {
		var o RechargeOrder
		if err := tx.First(&o, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRechargeOrderNotFound
			}
			return err
		}
		// Expire stale pending/processing orders first.
		now := time.Now().Unix()
		if (o.Status == RechargePending || o.Status == RechargeProcessing) && o.ExpiresAt > 0 && now > o.ExpiresAt {
			o.Status = RechargeExpired
			o.FailureReason = "payment_expired"
		}
		// Terminal states are idempotent: return the existing order.
		if o.Status == RechargePaid || o.Status == RechargeFailed ||
			o.Status == RechargeCancelled || o.Status == RechargeExpired {
			result = &o
			return nil
		}
		// Only pending/processing can transition.
		if o.Status != RechargePending && o.Status != RechargeProcessing {
			return fmt.Errorf("order %d is in state %s, cannot resolve", id, o.Status)
		}
		switch outcome {
		case "success":
			o.FundingSource = FundingSourceSimulation
			o.Status = RechargePaid
			o.PaidAt = now
			o.LatePayment = now > o.ExpiresAt
			o.FailureReason = ""
		case "failed":
			o.Status = RechargeFailed
			o.FailureReason = "payment_failed"
		case "cancelled":
			o.Status = RechargeCancelled
			o.FailureReason = "payment_cancelled"
		default:
			return fmt.Errorf("invalid outcome %q", outcome)
		}
		// Guarded update: only apply if the order is still in a
		// non-terminal state. This prevents a race between two
		// concurrent resolve calls.
		res := tx.Model(&RechargeOrder{}).
			Where("id = ? AND status IN ?", id, []string{RechargePending, RechargeProcessing}).
			Updates(map[string]any{
				"status":         o.Status,
				"paid_at":        o.PaidAt,
				"late_payment":   o.LatePayment,
				"failure_reason": o.FailureReason,
				"funding_source": o.FundingSource,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// Another resolve won the race; reload the current state.
			if err := tx.First(&o, "id = ?", id).Error; err != nil {
				return err
			}
			result = &o
			return nil
		}
		// On success, credit the wallet and write a ledger entry.
		if o.Status == RechargePaid {
			if err := tx.Model(&Wallet{}).Where("user_id = ?", o.UserID).
				Updates(map[string]any{"balance": gorm.Expr("balance + ?", o.Amount)}).Error; err != nil {
				return err
			}
			var w Wallet
			if err := tx.Where("user_id = ?", o.UserID).First(&w).Error; err != nil {
				return err
			}
			entry := &LedgerEntry{
				UserID:        o.UserID,
				Type:          "recharge",
				Amount:        o.Amount,
				BalanceAfter:  w.Balance,
				Description:   "Recharge order " + o.OrderNumber,
				OrderID:       o.OrderNumber,
				FundingSource: FundingSourceSimulation,
			}
			if err := tx.Create(entry).Error; err != nil {
				return err
			}
		}
		result = &o
		return nil
	})
	return result, err
}

// ExpireRechargeOrders marks all stale pending/processing orders as
// expired. Called lazily by the list and resolve handlers.
func ExpireRechargeOrders() error {
	now := time.Now().Unix()
	return DB.Model(&RechargeOrder{}).
		Where("status IN ? AND expires_at > 0 AND expires_at < ?",
			[]string{RechargePending, RechargeProcessing}, now).
		Updates(map[string]any{"status": RechargeExpired, "failure_reason": "payment_expired"}).Error
}

// BeginRechargePayment flips a pending order to processing. Returns the
// updated order. If the order is already processing or terminal, it is
// returned unchanged (idempotent).
func BeginRechargePayment(id int64) (*RechargeOrder, error) {
	var o RechargeOrder
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&o, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRechargeOrderNotFound
			}
			return err
		}
		if o.Status != RechargePending {
			return nil
		}
		now := time.Now().Unix()
		if o.ExpiresAt > 0 && now > o.ExpiresAt {
			o.Status = RechargeExpired
			o.FailureReason = "payment_expired"
			tx.Model(&RechargeOrder{}).Where("id = ?", id).
				Updates(map[string]any{"status": RechargeExpired, "failure_reason": "payment_expired"})
			return nil
		}
		tx.Model(&RechargeOrder{}).Where("id = ? AND status = ?", id, RechargePending).
			Updates(map[string]any{"status": RechargeProcessing})
		o.Status = RechargeProcessing
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// GenerateRechargeOrderNumber returns a human-readable order number.
func GenerateRechargeOrderNumber() string {
	return generateRechargeOrderNumberAt(time.Now())
}

func generateRechargeOrderNumberAt(now time.Time) string {
	return fmt.Sprintf("ord_%d_%s", now.Unix(), rand.Text())
}
