package model

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"gorm.io/gorm"
	"starhub/common"
)

var ErrStaffSecurity = errors.New("staff verification failed or expired")
var ErrLastRoot = errors.New("the last active Root account cannot be disabled or demoted")

func NewTOTPSecret() (string, error) {
	value := make([]byte, 20)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(value), nil
}

// TOTPCode implements RFC 6238 with six digits and a 30-second period.
func TOTPCode(secret string, step int64) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil || len(key) < 20 || step < 0 {
		return "", ErrStaffSecurity
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	h := hmac.New(sha1.New, key)
	_, _ = h.Write(counter[:])
	sum := h.Sum(nil)
	offset := sum[len(sum)-1] & 15
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1000000), nil
}

func validTOTPStep(secret, code string, last int64, now time.Time) (int64, bool) {
	if len(code) != 6 || strings.Trim(code, "0123456789") != "" {
		return 0, false
	}
	for _, delta := range []int64{0, -1, 1} {
		step := now.Unix()/30 + delta
		if step <= last {
			continue
		}
		want, err := TOTPCode(secret, step)
		if err == nil && subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

func sealStaffSecret(tx *gorm.DB, id int64, secret string) (string, error) {
	aead, err := tokenCipher(tx)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(secret), []byte(fmt.Sprintf("staff-totp:%d", id)))), nil
}

func openStaffSecret(tx *gorm.DB, user *User) (string, error) {
	aead, err := tokenCipher(tx)
	if err != nil {
		return "", err
	}
	data, err := base64.RawStdEncoding.DecodeString(user.TOTPSecret)
	if err != nil || len(data) < aead.NonceSize()+aead.Overhead() {
		return "", ErrStaffSecurity
	}
	value, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], []byte(fmt.Sprintf("staff-totp:%d", user.ID)))
	return string(value), err
}

// VerifyStaffOTP serializes enrollment, replay protection and account throttling.
// Only an unexpired password-authenticated session may supply enrollmentSecret.
func VerifyStaffOTP(id, version int64, code, enrollmentSecret string, now time.Time) (*User, error) {
	var result User
	verified := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&StaffAccount{}).Where("id = ?", id).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		user, err := staffUser(tx, id)
		if err != nil {
			return err
		}
		result = *user
		if result.AccountKind != "staff" || !common.IsStaffRole(result.Role) || result.Status != common.StatusEnabled || result.SecurityVersion != version || result.MustChangePassword || result.OTPLockedUntil > now.Unix() {
			return ErrStaffSecurity
		}
		secret := enrollmentSecret
		if result.TOTPSecret != "" {
			if secret != "" {
				return ErrStaffSecurity
			}
			var err error
			secret, err = openStaffSecret(tx, &result)
			if err != nil {
				return err
			}
		} else if secret == "" {
			return ErrStaffSecurity
		}
		step, ok := validTOTPStep(secret, code, result.TOTPLastStep, now)
		if !ok {
			failures := result.OTPFailures + 1
			until := int64(0)
			if failures >= 5 {
				until = now.Add(5 * time.Minute).Unix()
				failures = 0
			}
			return tx.Model(&StaffAccount{}).Where("id = ?", id).Updates(map[string]any{"otp_failures": failures, "otp_locked_until": until}).Error
		}
		fields := map[string]any{"totp_last_step": step, "otp_failures": 0, "otp_locked_until": 0}
		if result.TOTPSecret == "" {
			sealed, err := sealStaffSecret(tx, id, secret)
			if err != nil {
				return err
			}
			fields["totp_secret"] = sealed
		}
		if err := tx.Model(&StaffAccount{}).Where("id = ?", id).Updates(fields).Error; err != nil {
			return err
		}
		verified = true
		fresh, err := staffUser(tx, id)
		if err == nil {
			result = *fresh
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if !verified {
		return nil, ErrStaffSecurity
	}
	return &result, nil
}

func ChangeStaffPassword(id, version int64, currentPassword, newPassword string) (*User, error) {
	if len(newPassword) < 12 || len(newPassword) > 72 || newPassword == currentPassword {
		return nil, errors.New("use a different password of 12 to 72 bytes")
	}
	user, err := GetStaffByID(id)
	if err != nil || user.AccountKind != "staff" || user.Status != common.StatusEnabled || user.SecurityVersion != version || !common.VerifyPassword(currentPassword, user.Password) {
		return nil, ErrStaffSecurity
	}
	hash, err := common.HashPassword(newPassword)
	if err != nil {
		return nil, err
	}
	res := DB.Model(&StaffAccount{}).Where("id = ? AND security_version = ? AND password = ? AND status = ?", id, version, user.Password, common.StatusEnabled).Updates(map[string]any{"password": hash, "must_change_password": false, "security_version": gorm.Expr("security_version + 1")})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, ErrStaffSecurity
	}
	return GetStaffByID(id)
}

// UpdateStaffAccount retains disabled accounts and atomically protects the last Root.
func UpdateStaffAccount(id, version int64, role, status int, resetOTP bool, actor int64, reason string, contexts ...context.Context) error {
	if !common.IsStaffRole(role) || (status != common.StatusEnabled && status != common.StatusDisabled) {
		return errors.New("invalid staff role or status")
	}
	return adminRequestDB(contexts).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&StaffAccount{}).Where("id > 0").UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		user, err := staffUser(tx, id)
		if err != nil {
			return err
		}
		if user.SecurityVersion != version {
			return ErrStaffSecurity
		}
		if user.Role == common.RoleRoot && user.Status == common.StatusEnabled && (role != common.RoleRoot || status != common.StatusEnabled) {
			var roots int64
			if err := tx.Model(&StaffAccount{}).Where("role = ? AND status = ?", common.RoleRoot, common.StatusEnabled).Count(&roots).Error; err != nil {
				return err
			}
			if roots < 2 {
				return ErrLastRoot
			}
		}
		before := map[string]any{"platform_admin": user.HasPlatformWorkspace(), "role": user.Role, "status": user.Status, "two_factor_bound": user.TOTPSecret != ""}
		fields := map[string]any{"role": role, "status": status, "security_version": gorm.Expr("security_version + 1")}
		if resetOTP {
			fields["totp_secret"] = ""
			fields["totp_last_step"] = 0
			fields["otp_failures"] = 0
			fields["otp_locked_until"] = 0
		}
		if err := tx.Model(&StaffAccount{}).Where("id = ?", id).Updates(fields).Error; err != nil {
			return err
		}
		// Keep the business owner gate in sync for existing API keys and billing.
		if err := tx.Model(&User{}).Where("id = ? AND account_kind = ?", id, "staff").Updates(map[string]any{"role": role, "status": status}).Error; err != nil {
			return err
		}
		action := "staff_updated"
		if resetOTP {
			action = "staff_2fa_reset"
		}
		return writeStaffAudit(tx, actor, id, action, reason, "", before, map[string]any{"platform_admin": user.HasPlatformWorkspace(), "role": role, "status": status, "two_factor_bound": !resetOTP && user.TOTPSecret != ""})
	})
}

func adminRequestDB(contexts []context.Context) *gorm.DB {
	if len(contexts) > 0 {
		return DB.WithContext(contexts[0])
	}
	return DB
}

func writeStaffAudit(tx *gorm.DB, actor, target int64, action, reason, amount string, snapshots ...any) error {
	name := "deployment-cli"
	role := common.RoleRoot
	if actor > 0 {
		user, err := staffUser(tx, actor)
		if err != nil {
			return err
		}
		name, role = user.Username, user.Role
	}
	before, after := "", ""
	if len(snapshots) > 0 {
		raw, _ := json.Marshal(snapshots[0])
		before = string(raw)
	}
	if len(snapshots) > 1 {
		raw, _ := json.Marshal(snapshots[1])
		after = string(raw)
	}
	kind := "staff"
	if strings.HasPrefix(action, "customer_") || strings.HasPrefix(action, "balance_") {
		kind = "customers"
	}
	if action == "customer_key_disabled" {
		kind = "keys"
	}
	ip, _ := tx.Statement.Context.Value(common.CtxAdminIP).(string)
	return tx.Create(&AuditLog{IP: ip, ObjectType: kind, ObjectID: fmt.Sprint(target), BeforeSummary: before, AfterSummary: after, UserID: actor, Username: name, ActorRole: role, Action: action, TargetID: target, Reason: reason, AmountNano: amount, Method: "POST", Path: "/api/admin/staff", Status: 200}).Error
}

func CreateStaffAccount(user *User, actor int64, reason string, contexts ...context.Context) error {
	user.AccountKind = "staff"
	return adminRequestDB(contexts).Transaction(func(tx *gorm.DB) error {
		if err := createStaffIdentity(tx, user); err != nil {
			return err
		}
		return writeStaffAudit(tx, actor, user.ID, "staff_created", reason, "", nil, map[string]any{"username": user.Username, "platform_admin": user.HasPlatformWorkspace(), "role": user.Role, "status": user.Status, "two_factor_bound": false})
	})
}
