package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestGroupAccountReplacementValidatesUsersWithoutMutatingConfiguration(t *testing.T) {
	groupConfigurationDB(t)
	for _, id := range []int64{1, 2, 9223372036854775807} {
		if err := CreateUser(&User{ID: id, Username: fmt.Sprintf("member-%d", id), AffCode: fmt.Sprintf("code-%d", id)}); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	for _, value := range []string{"", "acct-1", "01", "+1", "1.0", "1e0", " 1", "1 ", "0", "-1", "9223372036854775808", "1,1", "1,3", strings.Repeat("1,", 100) + "2", strings.Repeat("9", 1025)} {
		before := groupConfigurationSnapshot(t)
		group := &ServiceGroupConfig{GroupID: "private", Enabled: true, Visibility: "custom", AccountIDs: value, Multiplier: "0.5"}
		if err := SaveServiceGroupAndRatio(ctx, group, 1); !errors.Is(err, ErrInvalidServiceGroupAccounts) {
			t.Fatalf("accepted invalid list %q: %v", value, err)
		}
		if groupConfigurationSnapshot(t) != before {
			t.Fatalf("invalid list %q changed storage", value)
		}
	}
	group := &ServiceGroupConfig{GroupID: "private", Enabled: true, Visibility: "custom", AccountIDs: "1,9223372036854775807", Multiplier: "0.5"}
	if err := SaveServiceGroupAndRatioIfRevision(ctx, group, 1, 0, 0); err != nil {
		t.Fatal(err)
	}
	before := groupConfigurationSnapshot(t)
	invalid := *group
	invalid.AccountIDs = "2,missing"
	if err := SaveServiceGroupAndRatioIfRevision(ctx, &invalid, 1, group.ID, group.Revision); !errors.Is(err, ErrInvalidServiceGroupAccounts) {
		t.Fatal(err)
	}
	if groupConfigurationSnapshot(t) != before {
		t.Fatal("invalid replacement changed membership, ratio or history")
	}
	replaced := *group
	replaced.AccountIDs = "2"
	if err := SaveServiceGroupAndRatioIfRevision(ctx, &replaced, 1, group.ID, group.Revision); err != nil {
		t.Fatal(err)
	}
	if err := SaveServiceGroupAndRatioIfRevision(ctx, group, 1, group.ID, group.Revision); !errors.Is(err, ErrServiceGroupConflict) {
		t.Fatalf("stale list edit overwrote the replacement: %v", err)
	}
}

func TestGroupAccountLegacyListCanBeRetainedButCannotBeReintroduced(t *testing.T) {
	groupConfigurationDB(t)
	group := &ServiceGroupConfig{GroupID: "legacy", Enabled: true, Visibility: "custom", AccountIDs: "acct-1,unknown", Multiplier: "0.5"}
	if err := UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	group.Name = "Repair other metadata without inventing membership"
	if err := SaveServiceGroupAndRatio(context.Background(), group, 1); err != nil {
		t.Fatal(err)
	}
	access, err := ServiceGroupAccess(context.Background(), []string{group.GroupID}, 1)
	if err != nil || access[group.GroupID] {
		t.Fatal("retaining unknown identifiers granted a real user access")
	}
	group.Visibility = "common"
	if err := SaveServiceGroupAndRatio(context.Background(), group, 1); err != nil {
		t.Fatal(err)
	}
	group.Visibility = "custom"
	if err := SaveServiceGroupAndRatio(context.Background(), group, 1); !errors.Is(err, ErrInvalidServiceGroupAccounts) {
		t.Fatalf("unknown historical identifiers were reintroduced: %v", err)
	}
}
