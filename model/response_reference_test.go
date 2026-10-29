package model

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

func referenceFixture() (ResponseReference, time.Time) {
	at := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	ch := &Channel{ID: 9, Type: 1, BaseURL: "https://upstream.example.invalid/v1", Key: "synthetic-one\nsynthetic-two"}
	return ResponseReference{UserID: 7, TokenID: 8, ResponseID: "resp_reference", ChannelID: ch.ID,
		CredentialIdentity: ResponseCredentialIdentity(ch, "synthetic-one"), Model: "gpt-4o", Group: "default", GroupDefinitionID: 3,
		AccountIdentity: ResponseAccountIdentity(ch, "synthetic-one"),
		Status:          "completed", PromptTokens: 100, CompletionTokens: 20, HasTools: true}, at
}

func TestResponseCredentialIdentityPinsExactAccountWithoutPlaintext(t *testing.T) {
	ch := &Channel{ID: 9, Type: 1, BaseURL: "https://upstream.example.invalid/v1", Key: "synthetic-one\nsynthetic-two"}
	identity := ResponseCredentialIdentity(ch, "synthetic-one")
	if len(identity) != 64 || strings.Contains(identity, "synthetic") {
		t.Fatal("credential identity must be an opaque digest")
	}
	ch.Key, ch.Weight, ch.UsedQuota, ch.KeyMode = "synthetic-three\nsynthetic-one", 99, 1000, KeyModeRandom
	if ResponseCredentialIdentity(ch, "synthetic-one") != identity {
		t.Fatal("pool scheduling metadata changed the selected account identity")
	}
	for _, mutate := range []func(*Channel){func(c *Channel) { c.ID++ }, func(c *Channel) { c.Type++ }, func(c *Channel) { c.BaseURL += "/other" }} {
		copy := *ch
		mutate(&copy)
		if ResponseCredentialIdentity(&copy, "synthetic-one") == identity {
			t.Fatal("upstream identity change did not change the digest")
		}
	}
	if ResponseCredentialIdentity(ch, "synthetic-two") == identity || ResponseCredentialIdentity(nil, "key") != "" || ResponseCredentialIdentity(ch, "") != "" {
		t.Fatal("selected pool credentials were conflated")
	}
	if ResponseCredentialIdentity(ch, string([]byte{0xff})) == ResponseCredentialIdentity(ch, string([]byte{0xfe})) {
		t.Fatal("distinct opaque credential bytes were replaced by the same JSON text")
	}
}

func TestResponseCredentialNeverRotatesAwayFromTheRecordedEnabledAccount(t *testing.T) {
	ch := &Channel{ID: 9, Type: 1, BaseURL: "https://upstream.example.invalid", Key: "first\nsecond", Status: 1}
	identity := ResponseCredentialIdentity(ch, "first")
	ch.Key, ch.KeyMode = "second\nfirst", KeyModeRandom
	if key, ok := ResponseCredential(ch, identity); !ok || key != "first" {
		t.Fatal("pool reorder selected a different upstream account")
	}
	for _, mutate := range []func(*Channel){func(c *Channel) { c.Key = "second" }, func(c *Channel) { c.MultiKeyStatus = "1,2" }, func(c *Channel) { c.Status = 2 }, func(c *Channel) { c.BaseURL += "/other" }} {
		copy := *ch
		mutate(&copy)
		if key, ok := ResponseCredential(&copy, identity); ok || key != "" {
			t.Fatal("unavailable recorded account fell back to another pool key")
		}
	}
}

func TestResponseAccountIdentitySpansDuplicateChannelsButNotDifferentCredentials(t *testing.T) {
	ch := &Channel{ID: 9, Type: 1, BaseURL: "https://upstream.example.invalid/v1/"}
	account := ResponseAccountIdentity(ch, "synthetic-one")
	ch.ID++
	ch.BaseURL = strings.TrimRight(ch.BaseURL, "/")
	if len(account) != 64 || ResponseAccountIdentity(ch, "synthetic-one") != account {
		t.Fatal("duplicate channel configuration created a different storage namespace")
	}
	if ResponseAccountIdentity(ch, "synthetic-two") == account || ResponseAccountIdentity(ch, string([]byte{0xff})) == ResponseAccountIdentity(ch, string([]byte{0xfe})) {
		t.Fatal("different opaque upstream credentials shared a storage namespace")
	}
	if ResponseAccountIdentity(nil, "key") != "" || ResponseAccountIdentity(ch, "") != "" {
		t.Fatal("missing account identity was accepted")
	}
}

func TestResponseReferenceScopeExpiryImmutabilityAndRestart(t *testing.T) {
	path := groupConfigurationDB(t)
	ctx := context.Background()
	ref, at := referenceFixture()
	ref.ID, ref.CreatedAt, ref.ExpiresAt, ref.BindingID = 999, 1, math.MaxInt64, "invented"
	stored, err := RecordResponseReference(ctx, ref, at)
	if err != nil || stored.ID == 999 || stored.CreatedAt != at.Unix() || stored.ExpiresAt != at.Add(ResponseReferenceLifetime).Unix() || len(stored.BindingID) != 64 {
		t.Fatalf("gateway-owned identity/deadline: %+v %v", stored, err)
	}
	raw, err := json.Marshal(stored)
	if err != nil || string(raw) != "{}" {
		t.Fatal("private reference metadata must not be serialized to a client")
	}
	for _, scope := range [][2]int64{{0, ref.TokenID}, {ref.UserID, 0}, {ref.UserID + 1, ref.TokenID}, {ref.UserID, ref.TokenID + 1}} {
		if _, err := LookupResponseReference(ctx, scope[0], scope[1], ref.ResponseID, at); !errors.Is(err, ErrResponseReferenceNotFound) {
			t.Fatal("another account/token could resolve a reference")
		}
	}
	for _, lookupAt := range []time.Time{at.Add(-time.Second), at.Add(ResponseReferenceLifetime), at.Add(ResponseReferenceLifetime + time.Second)} {
		if _, err := LookupResponseReference(ctx, ref.UserID, ref.TokenID, ref.ResponseID, lookupAt); !errors.Is(err, ErrResponseReferenceNotFound) {
			t.Fatal("reference was accessible outside its exclusive lifetime")
		}
	}
	if found, err := LookupResponseReference(ctx, ref.UserID, ref.TokenID, ref.ResponseID, at.Add(ResponseReferenceLifetime-time.Second)); err != nil || *found != *stored {
		t.Fatalf("valid owner lost the original binding: %+v %v", found, err)
	}
	for _, repeatAt := range []time.Time{at.Add(24 * time.Hour), at.Add(ResponseReferenceLifetime + 24*time.Hour)} {
		repeated, err := RecordResponseReference(ctx, ref, repeatAt)
		if err != nil || *repeated != *stored {
			t.Fatal("duplicate observation overwrote the reference or renewed its lifetime")
		}
	}
	changed := ref
	changed.PromptTokens++
	if _, err := RecordResponseReference(ctx, changed, at); !errors.Is(err, ErrResponseReferenceConflict) {
		t.Fatal("duplicate ID overwrote immutable accounting metadata")
	}
	if _, err := LookupResponseReference(ctx, ref.UserID, ref.TokenID, ref.ResponseID, at); !errors.Is(err, ErrResponseReferenceAmbiguous) {
		t.Fatal("conflicting metadata retained a readable ownership grant")
	}
	if _, err := RecordResponseReference(ctx, ref, at); !errors.Is(err, ErrResponseReferenceConflict) {
		t.Fatal("repeating the original metadata revived a conflicted grant")
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	if _, err := LookupResponseReference(ctx, ref.UserID, ref.TokenID, ref.ResponseID, at); !errors.Is(err, ErrResponseReferenceAmbiguous) {
		t.Fatal("restart lost the persistent collision refusal")
	}
	if version, err := OptionVersion(); err != nil || version != 0 {
		t.Fatal("recording references changed pricing configuration")
	}
}

func TestResponseReferenceReusedProviderIDsNeverSelectAnotherBinding(t *testing.T) {
	groupConfigurationDB(t)
	ctx := context.Background()
	ref, at := referenceFixture()
	if _, err := RecordResponseReference(ctx, ref, at); err != nil {
		t.Fatal(err)
	}
	otherToken := ref
	otherToken.TokenID++
	otherToken.ChannelID++
	otherToken.AccountIdentity = strings.Repeat("c", 64)
	if _, err := RecordResponseReference(ctx, otherToken, at); err != nil {
		t.Fatal(err)
	}
	if found, err := LookupResponseReference(ctx, ref.UserID, ref.TokenID, ref.ResponseID, at); err != nil || found.ChannelID != ref.ChannelID {
		t.Fatal("another token's provider ID interfered with this token")
	}
	for index, mutate := range []func(*ResponseReference){
		func(r *ResponseReference) { r.ChannelID++ },
		func(r *ResponseReference) { r.CredentialIdentity = strings.Repeat("b", 64) },
		func(r *ResponseReference) { r.Model = "other-model" },
		func(r *ResponseReference) { r.Group = "other-group" },
		func(r *ResponseReference) { r.GroupDefinitionID++ },
	} {
		changed := ref
		changed.ResponseID += string(rune('a' + index))
		if _, err := RecordResponseReference(ctx, changed, at); err != nil {
			t.Fatal(err)
		}
		mutate(&changed)
		if _, err := RecordResponseReference(ctx, changed, at.Add(ResponseReferenceLifetime)); err != nil {
			t.Fatal(err)
		}
		for _, lookupAt := range []time.Time{at, at.Add(ResponseReferenceLifetime + time.Second)} {
			if _, err := LookupResponseReference(ctx, changed.UserID, changed.TokenID, changed.ResponseID, lookupAt); !errors.Is(err, ErrResponseReferenceAmbiguous) {
				t.Fatal("reused ID selected a different binding, including after the first binding expired")
			}
		}
	}
}

func TestResponseReferenceRejectsMalformedMetadataAndRollsBackFailures(t *testing.T) {
	groupConfigurationDB(t)
	ctx := context.Background()
	ref, at := referenceFixture()
	for _, mutate := range []func(*ResponseReference){
		func(r *ResponseReference) { r.UserID = 0 }, func(r *ResponseReference) { r.TokenID = -1 }, func(r *ResponseReference) { r.ChannelID = 0 },
		func(r *ResponseReference) { r.ResponseID = "../other" }, func(r *ResponseReference) { r.ResponseID = "resp?key=secret" }, func(r *ResponseReference) { r.ResponseID = strings.Repeat("r", 257) },
		func(r *ResponseReference) { r.CredentialIdentity = "synthetic-plaintext-key" }, func(r *ResponseReference) { r.CredentialIdentity = strings.Repeat("F", 64) },
		func(r *ResponseReference) { r.Model = " m" }, func(r *ResponseReference) { r.Group = "g\n" }, func(r *ResponseReference) { r.GroupDefinitionID = -1 },
		func(r *ResponseReference) { r.Status = "queued" }, func(r *ResponseReference) { r.PromptTokens = -1 }, func(r *ResponseReference) { r.CompletionTokens = -1 },
		func(r *ResponseReference) { r.PromptTokens = math.MaxInt64 },
	} {
		invalid := ref
		mutate(&invalid)
		if _, err := RecordResponseReference(ctx, invalid, at); !errors.Is(err, ErrInvalidResponseReference) {
			t.Fatalf("malformed metadata accepted: %+v %v", invalid, err)
		}
	}
	for _, invalidAt := range []time.Time{time.Time{}, time.Unix(math.MaxInt64, 0)} {
		if _, err := RecordResponseReference(ctx, ref, invalidAt); !errors.Is(err, ErrInvalidResponseReference) {
			t.Fatal("invalid/overflowing reference deadline accepted")
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := RecordResponseReference(cancelled, ref, at); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled reference write was accepted")
	}
	if err := DB.Exec(`CREATE TRIGGER reject_response_reference BEFORE INSERT ON response_references BEGIN SELECT RAISE(ABORT, 'synthetic reference failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := RecordResponseReference(ctx, ref, at); err == nil {
		t.Fatal("reference storage failure was ignored")
	}
	var count int64
	if err := DB.Model(&ResponseReference{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("invalid/cancelled/failed writes leaked references")
	}
}

func TestResponseReferenceConcurrentDuplicateObservationsStayOneRow(t *testing.T) {
	groupConfigurationDB(t)
	ref, at := referenceFixture()
	var writers sync.WaitGroup
	ids := make(chan int64, 12)
	for index := 0; index < 12; index++ {
		writers.Go(func() {
			stored, err := RecordResponseReference(context.Background(), ref, at)
			if err != nil {
				t.Error(err)
				return
			}
			ids <- stored.ID
		})
	}
	writers.Wait()
	close(ids)
	var first int64
	written := 0
	for id := range ids {
		if first == 0 {
			first = id
		}
		if id != first {
			t.Fatal("concurrent duplicate observations created different bindings")
		}
		written++
	}
	var count int64
	if err := DB.Model(&ResponseReference{}).Count(&count).Error; err != nil || count != 1 || written != 12 {
		t.Fatal("duplicate reference observations were not atomic")
	}
}
