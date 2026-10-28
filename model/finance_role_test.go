package model

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"starhub/common"
)

func TestFinanceRoleConcurrentStaleGrantsCannotOverwriteWinner(t *testing.T) {
	newTestDB(t)
	u := &User{Username: "finance-concurrent", Role: common.RoleUser, Status: common.StatusEnabled}
	if err := CreateUser(u); err != nil {
		t.Fatal(err)
	}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := SetUserFinanceRole(u.ID, true, common.RoleUser)
			if err == nil {
				wins.Add(1)
			} else if !errors.Is(err, ErrUserRoleConflict) {
				t.Errorf("unexpected grant error: %v", err)
			}
		}()
	}
	wg.Wait()
	after, err := GetUserByID(u.ID)
	if err != nil || wins.Load() != 1 || after.Role != common.RoleFinance {
		t.Fatalf("concurrent grant: wins=%d user=%+v err=%v", wins.Load(), after, err)
	}
}
