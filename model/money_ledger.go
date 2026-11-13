package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"gorm.io/gorm"
	"starhub/common"
)

var ErrMoneyConflict = errors.New("accounting balance or operation changed")

// MoneyAccount is the current head of the canonical nano-USD journal. Available
// and spent values must match the exact compatibility balance representation.
type MoneyAccount struct {
	UserID        int64  `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	Sequence      int64  `gorm:"not null" json:"sequence"`
	AvailableNano string `gorm:"size:64;not null" json:"available_nano"`
	ReservedNano  string `gorm:"size:64;not null" json:"reserved_nano"`
	SpentNano     string `gorm:"size:64;not null" json:"spent_nano"`
	CreatedAt     int64  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     int64  `gorm:"autoUpdateTime" json:"updated_at"`
}

type MoneyEntry struct {
	ID                 int64  `gorm:"primaryKey" json:"id"`
	UserID             int64  `gorm:"index;uniqueIndex:idx_money_sequence;uniqueIndex:idx_money_operation" json:"user_id"`
	Sequence           int64  `gorm:"uniqueIndex:idx_money_sequence" json:"sequence"`
	OperationID        string `gorm:"size:192;uniqueIndex:idx_money_operation" json:"operation_id"`
	RequestDigest      string `gorm:"size:64" json:"-"`
	Kind               string `gorm:"size:32" json:"type"`
	AvailableDeltaNano string `gorm:"size:64;not null" json:"available_delta_nano"`
	ReservedDeltaNano  string `gorm:"size:64;not null" json:"reserved_delta_nano"`
	SpentDeltaNano     string `gorm:"size:64;not null" json:"spent_delta_nano"`
	AvailableAfterNano string `gorm:"size:64;not null" json:"available_after_nano"`
	ReservedAfterNano  string `gorm:"size:64;not null" json:"reserved_after_nano"`
	SpentAfterNano     string `gorm:"size:64;not null" json:"spent_after_nano"`
	ReservationID      string `gorm:"size:64;index" json:"reservation_id,omitempty"`
	LogID              int64  `gorm:"index" json:"log_id,omitempty"`
	ActorID            int64  `json:"actor_id,omitempty"`
	Reason             string `gorm:"size:256" json:"reason"`
	FundingSource      string `gorm:"size:32" json:"funding_source"`
	CreatedAt          int64  `gorm:"autoCreateTime;index" json:"created_at"`
}

func nanoAdmission(id string) bool { return strings.HasPrefix(id, "n2:") }

func pendingUserMoney(tx *gorm.DB, userID int64) (*big.Int, error) {
	amount := new(big.Int)
	var reservations []QuotaReservation
	if err := tx.Where("user_id = ? AND completed_at = 0", userID).Find(&reservations).Error; err != nil {
		return nil, err
	}
	for _, r := range reservations {
		if r.Amount < 0 {
			return nil, ErrQuotaReservation
		}
		value := big.NewInt(r.Amount)
		if !nanoAdmission(r.ID) {
			value = common.QuotaMoney(r.Amount, 0)
		}
		amount.Add(amount, value)
	}
	var tasks []Task
	if err := tx.Where("user_id = ? AND quota_reservation_id = '' AND status NOT IN ?", userID, []string{"succeeded", "failed", "cancelled"}).Find(&tasks).Error; err != nil {
		return nil, err
	}
	for _, task := range tasks {
		if task.Quota < 0 {
			return nil, ErrQuotaReservation
		}
		amount.Add(amount, common.QuotaMoney(task.Quota, 0))
	}
	var intents []QuotaSettlement
	if err := tx.Where("user_id = ? AND completed_at = 0", userID).Find(&intents).Error; err != nil {
		return nil, err
	}
	for _, intent := range intents {
		var entry Log
		if json.Unmarshal([]byte(intent.Payload), &entry) != nil {
			return nil, ErrMoneyConflict
		}
		if entry.QuotaReservationID != "" {
			continue
		}
		held := new(big.Int).Sub(big.NewInt(intent.Actual), big.NewInt(intent.Delta))
		if held.Sign() < 0 {
			return nil, ErrMoneyConflict
		}
		held.Mul(held, big.NewInt(common.NanoPerLegacyQuota))
		amount.Add(amount, held)
	}
	return amount, nil
}

// ensureMoneyAccount runs after acquiring the user's SQLite writer lock. The
// first journal entry records a verified snapshot, not a fabricated payment.
func ensureMoneyAccount(tx *gorm.DB, user User) (*MoneyAccount, error) {
	available := common.QuotaMoney(user.Quota, user.QuotaRemainderNano)
	spent := common.QuotaMoney(user.UsedQuota, user.UsedQuotaRemainderNano)
	if available.Sign() < 0 || spent.Sign() < 0 {
		return nil, ErrMoneyConflict
	}
	var account MoneyAccount
	err := tx.First(&account, "user_id = ?", user.ID).Error
	if err == nil {
		for _, raw := range []string{account.AvailableNano, account.ReservedNano, account.SpentNano} {
			value, err := common.ParseNano(raw)
			if err != nil || value.Sign() < 0 {
				return nil, ErrMoneyConflict
			}
		}
		if account.AvailableNano != available.String() || account.SpentNano != spent.String() {
			return nil, ErrMoneyConflict
		}
		return &account, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	held, err := pendingUserMoney(tx, user.ID)
	if err != nil {
		return nil, err
	}
	account = MoneyAccount{UserID: user.ID, Sequence: 1, AvailableNano: available.String(), ReservedNano: held.String(), SpentNano: spent.String()}
	if err := tx.Create(&account).Error; err != nil {
		return nil, err
	}
	entry := MoneyEntry{UserID: user.ID, Sequence: 1, OperationID: "opening", Kind: "opening", AvailableDeltaNano: available.String(), ReservedDeltaNano: held.String(), SpentDeltaNano: spent.String(), AvailableAfterNano: available.String(), ReservedAfterNano: held.String(), SpentAfterNano: spent.String(), FundingSource: "legacy_quota_opening", Reason: "Opening snapshot of existing spendable quota and durable holds"}
	if err := tx.Create(&entry).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

type moneyMovement struct {
	Available                                                       *big.Int
	Reserved                                                        *big.Int
	Spent                                                           *big.Int
	Kind, OperationID, Digest, ReservationID, Reason, FundingSource string
	ActorID, LogID                                                  int64
}

func recordMoneyMovement(tx *gorm.DB, account *MoneyAccount, m moneyMovement) (*MoneyEntry, error) {
	if m.OperationID == "" || len(m.OperationID) > 192 || len(m.Reason) > 256 {
		return nil, ErrMoneyConflict
	}
	values := []string{account.AvailableNano, account.ReservedNano, account.SpentNano}
	deltas := []*big.Int{m.Available, m.Reserved, m.Spent}
	after := make([]string, 3)
	changes := make([]string, 3)
	for i, raw := range values {
		value, err := common.ParseNano(raw)
		if err != nil {
			return nil, err
		}
		delta := deltas[i]
		if delta == nil {
			delta = new(big.Int)
		}
		changes[i] = delta.String()
		value.Add(value, delta)
		if value.Sign() < 0 {
			return nil, ErrMoneyConflict
		}
		after[i] = value.String()
	}
	var user User
	if err := tx.First(&user, account.UserID).Error; err != nil {
		return nil, err
	}
	if common.QuotaMoney(user.Quota, user.QuotaRemainderNano).String() != after[0] || common.QuotaMoney(user.UsedQuota, user.UsedQuotaRemainderNano).String() != after[2] {
		return nil, ErrMoneyConflict
	}
	entry := &MoneyEntry{UserID: account.UserID, Sequence: account.Sequence + 1, OperationID: m.OperationID, RequestDigest: m.Digest, Kind: m.Kind, AvailableDeltaNano: changes[0], ReservedDeltaNano: changes[1], SpentDeltaNano: changes[2], AvailableAfterNano: after[0], ReservedAfterNano: after[1], SpentAfterNano: after[2], ReservationID: m.ReservationID, LogID: m.LogID, ActorID: m.ActorID, Reason: m.Reason, FundingSource: m.FundingSource}
	if err := tx.Create(entry).Error; err != nil {
		return nil, err
	}
	account.Sequence++
	account.AvailableNano, account.ReservedNano, account.SpentNano = after[0], after[1], after[2]
	if err := tx.Save(account).Error; err != nil {
		return nil, err
	}
	return entry, nil
}

func lockMoneyUser(tx *gorm.DB, userID int64) (User, *MoneyAccount, error) {
	updated := tx.Model(&User{}).Where("id = ?", userID).UpdateColumn("quota", gorm.Expr("quota"))
	if updated.Error != nil {
		return User{}, nil, updated.Error
	}
	if updated.RowsAffected != 1 {
		return User{}, nil, ErrUserNotFound
	}
	var user User
	if err := tx.First(&user, userID).Error; err != nil {
		return user, nil, err
	}
	account, err := ensureMoneyAccount(tx, user)
	return user, account, err
}

func setMoneyPair(tx *gorm.DB, table any, id int64, field string, value *big.Int) error {
	major, remainder, err := common.NanoQuotaParts(value)
	if err != nil || value.Sign() < 0 {
		return ErrMoneyConflict
	}
	return tx.Model(table).Where("id = ?", id).Updates(map[string]any{field: major, field + "_remainder_nano": remainder}).Error
}

// VerifyMoneyLedger rebuilds every dimension from journal deltas in one read
// snapshot and reports drift without repairing or promoting unverified funds.
func VerifyMoneyLedger(userID int64) error {
	return DB.Transaction(func(tx *gorm.DB) error { return verifyMoneyLedger(tx, userID) })
}

func verifyMoneyLedger(tx *gorm.DB, userID int64) error {
	var account MoneyAccount
	if err := tx.First(&account, "user_id = ?", userID).Error; err != nil {
		return err
	}
	rows, err := tx.Model(&MoneyEntry{}).Where("user_id = ?", userID).Order("sequence").Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	sums := []*big.Int{new(big.Int), new(big.Int), new(big.Int)}
	sequence := int64(0)
	for rows.Next() {
		var entry MoneyEntry
		if err := tx.ScanRows(rows, &entry); err != nil {
			return err
		}
		sequence++
		if entry.Sequence != sequence {
			return ErrMoneyConflict
		}
		for i, raw := range []string{entry.AvailableDeltaNano, entry.ReservedDeltaNano, entry.SpentDeltaNano} {
			delta, err := common.ParseNano(raw)
			if err != nil {
				return err
			}
			sums[i].Add(sums[i], delta)
		}
		if sums[0].String() != entry.AvailableAfterNano || sums[1].String() != entry.ReservedAfterNano || sums[2].String() != entry.SpentAfterNano {
			return ErrMoneyConflict
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if account.Sequence != sequence || account.AvailableNano != sums[0].String() || account.ReservedNano != sums[1].String() || account.SpentNano != sums[2].String() {
		return ErrMoneyConflict
	}
	var user User
	if err := tx.First(&user, userID).Error; err != nil {
		return err
	}
	if common.QuotaMoney(user.Quota, user.QuotaRemainderNano).String() != account.AvailableNano || common.QuotaMoney(user.UsedQuota, user.UsedQuotaRemainderNano).String() != account.SpentNano {
		return fmt.Errorf("%w: user projection", ErrMoneyConflict)
	}
	held, err := pendingUserMoney(tx, userID)
	if err != nil {
		return err
	}
	if held.String() != account.ReservedNano {
		return fmt.Errorf("%w: pending reservation projection", ErrMoneyConflict)
	}
	return nil
}
