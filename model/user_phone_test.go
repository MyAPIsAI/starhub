package model

import (
	"strings"
	"testing"

	"starhub/common"
)

func TestNormalizePhone(t *testing.T) {
	cases := []struct {
		input string
		want  string
		err   bool
	}{
		{"13812345678", "+8613812345678", false},
		{"+8613812345678", "+8613812345678", false},
		{"008613812345678", "+8613812345678", false},
		{"86 138-1234-5678", "+8613812345678", false},
		{"+86 13812345678", "+8613812345678", false},
		{"12345678901", "", true},  // starts with 10 — not a valid China mobile
		{"1381234567", "", true},   // 10 digits — too short
		{"138123456789", "", true}, // 12 digits — too long
		{"", "", true},
		{"abc", "", true},
		{"+12125551234", "", true}, // US number — not supported
	}
	for _, tc := range cases {
		got, err := NormalizePhone(tc.input)
		if tc.err {
			if err == nil {
				t.Errorf("NormalizePhone(%q) expected error, got %q", tc.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizePhone(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("NormalizePhone(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestCreateAndGetUserPhoneIdentity(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "phone-test", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}

	phone := "+8613812345678"
	if err := CreateUserPhoneIdentity(nil, user.ID, phone, 1700000000); err != nil {
		t.Fatalf("CreateUserPhoneIdentity failed: %v", err)
	}

	// GetUserPhone
	identity, err := GetUserPhone(user.ID)
	if err != nil {
		t.Fatalf("GetUserPhone failed: %v", err)
	}
	if identity.PhoneE164 != phone {
		t.Fatalf("phone = %s, want %s", identity.PhoneE164, phone)
	}

	// GetUserByPhone
	found, err := GetUserByPhone(phone)
	if err != nil {
		t.Fatalf("GetUserByPhone failed: %v", err)
	}
	if found.ID != user.ID {
		t.Fatalf("user id = %d, want %d", found.ID, user.ID)
	}
}

func TestCreateUserPhoneIdentityDuplicatePhone(t *testing.T) {
	newTestDB(t)
	user1 := &User{Username: "phone-user1", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	user2 := &User{Username: "phone-user2", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	if err := CreateUser(user1); err != nil {
		t.Fatal(err)
	}
	if err := CreateUser(user2); err != nil {
		t.Fatal(err)
	}

	phone := "+8613912345678"
	if err := CreateUserPhoneIdentity(nil, user1.ID, phone, 1700000000); err != nil {
		t.Fatalf("first bind failed: %v", err)
	}
	// Same phone, different user → error.
	err := CreateUserPhoneIdentity(nil, user2.ID, phone, 1700000000)
	if err == nil || !strings.Contains(err.Error(), "already bound") {
		t.Fatalf("expected ErrPhoneAlreadyBound, got %v", err)
	}
}

func TestCreateUserPhoneIdentityIdempotent(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "phone-idem", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	phone := "+8613712345678"
	if err := CreateUserPhoneIdentity(nil, user.ID, phone, 1700000000); err != nil {
		t.Fatalf("first bind failed: %v", err)
	}
	// Same user, same phone → no error, no duplicate.
	if err := CreateUserPhoneIdentity(nil, user.ID, phone, 1700000001); err != nil {
		t.Fatalf("idempotent bind failed: %v", err)
	}
	var count int64
	DB.Model(&UserPhoneIdentity{}).Where("user_id = ?", user.ID).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}
}

func TestGetUserByPhoneNotFound(t *testing.T) {
	newTestDB(t)
	_, err := GetUserByPhone("+8613812345678")
	if err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

func TestGetUserPhoneNotFound(t *testing.T) {
	newTestDB(t)
	_, err := GetUserPhone(9999)
	if err != ErrUserPhoneNotFound {
		t.Fatalf("expected ErrUserPhoneNotFound, got %v", err)
	}
}

func TestDeleteUserPhone(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "phone-del", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	phone := "+8613612345678"
	if err := CreateUserPhoneIdentity(nil, user.ID, phone, 1700000000); err != nil {
		t.Fatal(err)
	}
	if err := DeleteUserPhone(nil, user.ID); err != nil {
		t.Fatalf("DeleteUserPhone failed: %v", err)
	}
	if _, err := GetUserPhone(user.ID); err != ErrUserPhoneNotFound {
		t.Fatalf("expected ErrUserPhoneNotFound after delete, got %v", err)
	}
}
