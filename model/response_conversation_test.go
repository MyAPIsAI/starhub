package model

import (
	"fmt"
	"sync"
	"testing"
)

func TestResponseConversationClaimsSerializeTurnsAndDoNotReviveDeletedIDs(t *testing.T) {
	newTestDB(t)
	u := &User{Username: "conv", Status: 1, AffCode: "conv"}
	CreateUser(u)
	conversation, err := CreateResponseConversation(u.ID, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wins := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			nonce := fmt.Sprint(i)
			if ClaimResponseConversation(conversation, nonce) == nil {
				wins <- nonce
			}
		}(i)
	}
	wg.Wait()
	close(wins)
	var winner string
	count := 0
	for nonce := range wins {
		winner = nonce
		count++
	}
	if count != 1 {
		t.Fatalf("conversation winners=%d", count)
	}
	if err := FinishResponseConversation(conversation.ID, winner, "resp_one"); err != nil {
		t.Fatal(err)
	}
	fresh, _ := GetResponseConversation(conversation.ID, u.ID, 1)
	if fresh.Busy || fresh.LastResponseID != "resp_one" {
		t.Fatal("conversation completion missing")
	}
	if err := ClaimResponseConversation(conversation, "stale"); err == nil {
		t.Fatal("stale conversation version reused")
	}
	if err := DeleteResponseConversation(fresh); err != nil {
		t.Fatal(err)
	}
	if _, err := GetResponseConversation(fresh.ID, u.ID, 1); err == nil {
		t.Fatal("deleted conversation revived")
	}
}
