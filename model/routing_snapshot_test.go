package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testRouteSnapshot(now time.Time) RouteSnapshot {
	s := RouteSnapshot{SnapshotVersion: "snapshot-1", ScoreAlgorithmVersion: "five-dimension-v1", RatingAlgorithmVersion: "un-in-v1", PublishedAt: now, ResourceCount: 1, EligibleCount: 1,
		Policy: json.RawMessage(`{"schema":1,"quality":{"enabled":true}}`), PolicyFingerprint: "policy-1",
		Items: []json.RawMessage{json.RawMessage(`{"resource_id":"1:2:t10","channel_id":"1","model_id":"2","model_name":"test-model","price_tier":10,"quality_level":"S1","state":"hot","trend":"stable","score":90,"dim_scores":{"success_rate":99,"cost":80,"speed":90,"health":99,"stability":98},"future_evidence":{"value":9007199254740993}}`)}}
	s.Checksum = s.PolicyChecksum()
	return s
}

func TestRoutingSnapshotAtomicApplicationAndRestartGate(t *testing.T) {
	newTestDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	s := testRouteSnapshot(now)
	if err := ApplyRoutingSnapshot(s, now); err != nil {
		t.Fatal(err)
	}
	var row RoutingSnapshotRecord
	if err := DB.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(row.Payload, "9007199254740993") {
		t.Fatal("unknown evidence lost integer precision")
	}
	if err := ApplyRoutingSnapshot(s, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var duplicate RoutingSnapshotRecord
	DB.First(&duplicate)
	if !duplicate.ReceivedAt.Equal(row.ReceivedAt) {
		t.Fatal("duplicate refreshed age")
	}
	tampered := s
	tampered.Items = []json.RawMessage{json.RawMessage(strings.Replace(string(s.Items[0]), `"score":90`, `"score":99`, 1))}
	if err := ApplyRoutingSnapshot(tampered, now); err == nil {
		t.Fatal("accepted tampered checksum")
	}
	tampered.Checksum = tampered.PolicyChecksum()
	if err := ApplyRoutingSnapshot(tampered, now); err == nil {
		t.Fatal("accepted conflicting version")
	}
	older := s
	older.SnapshotVersion, older.PublishedAt = "older", now.Add(-time.Second)
	if err := ApplyRoutingSnapshot(older, now); err == nil {
		t.Fatal("accepted stale snapshot")
	}
	version, _, ready := RoutingSnapshotStatus()
	if !ready || version != s.SnapshotVersion {
		t.Fatal("invalid update changed active snapshot")
	}
	ResetRoutingSnapshot()
	if _, _, ready := RoutingSnapshotStatus(); ready {
		t.Fatal("restart bypassed synchronization gate")
	}
	if err := ApplyRoutingSnapshot(s, now); err != nil {
		t.Fatal(err)
	}
	if _, _, ready := RoutingSnapshotStatus(); !ready {
		t.Fatal("publisher replay did not reopen gate")
	}
}

func TestRoutingSnapshotRejectsInvalidFields(t *testing.T) {
	now := time.Now()
	for _, replacement := range []struct{ from, to string }{
		{`"state":"hot"`, `"state":"unknown"`},
		{`"trend":"stable"`, `"trend":"unknown"`},
		{`"quality_level":"S1"`, `"quality_level":"unknown"`},
		{`"speed":90`, `"speed":-1`},
		{`"score":90`, `"score":null`},
		{`"channel_id":"1"`, `"channel_id":"2"`},
	} {
		s := testRouteSnapshot(now)
		s.Items[0] = json.RawMessage(strings.Replace(string(s.Items[0]), replacement.from, replacement.to, 1))
		s.Checksum = s.PolicyChecksum()
		if _, err := s.Validate(now); err == nil {
			t.Errorf("accepted %s", replacement.to)
		}
	}
}

func TestResourceCredentialRotationExpiryAndRevocation(t *testing.T) {
	newTestDB(t)
	old, next := strings.Repeat("a", 32), strings.Repeat("b", 32)
	t.Setenv("METRIC_AUTHORIZATION", old)
	now := time.Now().Truncate(time.Second)
	if !ResourceCredentialValid(old, now) || ResourceCredentialValid("", now) {
		t.Fatal("bootstrap authentication failed")
	}
	if err := RotateResourceCredential(next, false, now); err != nil {
		t.Fatal(err)
	}
	if !ResourceCredentialValid(old, now.Add(599*time.Second)) || !ResourceCredentialValid(next, now.Add(time.Hour)) {
		t.Fatal("rotation overlap failed")
	}
	if err := RotateResourceCredential(next, false, now.Add(599*time.Second)); err != nil {
		t.Fatal(err)
	}
	if ResourceCredentialValid(old, now.Add(600*time.Second)) {
		t.Fatal("old token survived fixed deadline")
	}
	if err := RotateResourceCredential("", true, now); err != nil {
		t.Fatal(err)
	}
	if ResourceCredentialValid(old, now) || ResourceCredentialValid(next, now) {
		t.Fatal("revoked credential still accepted")
	}
}
