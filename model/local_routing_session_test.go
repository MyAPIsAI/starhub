package model

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"starhub/internal/upstream"
)

func routingSessionFixture(t *testing.T) (*Channel, *RoutingAttempt) {
	t.Helper()
	_, _ = scoredFixture(t, 1)
	ch, err := GetSatisfiedChannel("default", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	if ch.RuntimeRoute == nil {
		t.Fatal("missing routing attempt")
	}
	return ch, ch.RuntimeRoute
}

func TestLocalRoutingSessionFollowsOwnFailures(t *testing.T) {
	_, session := routingSessionFixture(t)
	for range 7 {
		if err := RecordRoutingOutcome(session, false, 503, 0, time.Now(), true); err != nil {
			t.Fatal(err)
		}
		if err := RecordRoutingOutcome(session, true, 200, 0, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	state, err := ReadLocalRoutingState(session.Item)
	if err != nil {
		t.Fatal(err)
	}
	if state.ConsecutiveFailures != 0 || state.OpenUntil != 0 || session.failureSnapshotInvalid || session.FailureGeneration != state.FailureGeneration {
		t.Fatalf("own intermittent failures accumulated: state=%+v session=%+v", state, session)
	}
}

func TestLocalRoutingSessionPreservesExternalFailures(t *testing.T) {
	for _, beforeOwn := range []bool{true, false} {
		t.Run(map[bool]string{true: "external_before_own", false: "external_after_own"}[beforeOwn], func(t *testing.T) {
			_, session := routingSessionFixture(t)
			external := *session
			fail := func(attempt *RoutingAttempt, continuing bool) {
				t.Helper()
				if err := RecordRoutingOutcome(attempt, false, 503, 0, time.Now(), continuing); err != nil {
					t.Fatal(err)
				}
			}
			if beforeOwn {
				fail(&external, false)
			}
			fail(session, true)
			if !beforeOwn {
				fail(&external, false)
			}
			if err := RecordRoutingOutcome(session, true, 200, 0, time.Now()); err != nil {
				t.Fatal(err)
			}
			state, err := ReadLocalRoutingState(session.Item)
			if err != nil {
				t.Fatal(err)
			}
			if state.ConsecutiveFailures != 2 || !session.failureSnapshotInvalid {
				t.Fatalf("external failure was cleared: state=%+v session=%+v", state, session)
			}
			// Later own failures still persist, but cannot make a stale session
			// eligible to clear the external failure again.
			fail(session, true)
			if err := RecordRoutingOutcome(session, true, 200, 0, time.Now()); err != nil {
				t.Fatal(err)
			}
			state, err = ReadLocalRoutingState(session.Item)
			if err != nil {
				t.Fatal(err)
			}
			if state.ConsecutiveFailures != 3 {
				t.Fatalf("stale session resumed following: %+v", state)
			}
		})
	}
}

func TestLocalRoutingSessionDoesNotFollowRolledBackFailure(t *testing.T) {
	_, session := routingSessionFixture(t)
	if err := RecordRoutingOutcome(session, false, 503, 0, time.Now(), true); err != nil {
		t.Fatal(err)
	}
	generation := session.FailureGeneration
	external := *session
	if err := RecordRoutingOutcome(&external, false, 503, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := DB.Exec(`CREATE TRIGGER reject_local_failure BEFORE UPDATE ON local_routing_states BEGIN SELECT RAISE(ABORT, 'local state failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := RecordRoutingOutcome(session, false, 503, 0, time.Now(), true); err == nil {
		t.Fatal("expected persistence failure")
	}
	if session.FailureGeneration != generation || session.failureSnapshotInvalid {
		t.Fatalf("rolled-back result changed the session: %+v", session)
	}
	state, err := ReadLocalRoutingState(session.Item)
	if err != nil {
		t.Fatal(err)
	}
	if state.ConsecutiveFailures != 2 {
		t.Fatalf("failed transaction changed state: %+v", state)
	}
}

func TestFallbackRoutingSessionFollowsCommittedRecovery(t *testing.T) {
	ch, session := routingSessionFixture(t)
	session.OrdinaryFallback, session.Sent = true, true
	BeginRoutingAttempt(session)
	identity := UpstreamIdentity(ch, "test-model", "upstream")
	for range 3 {
		if err := RecordRoutingOutcome(session, false, 503, 0, time.Now(), true); err != nil {
			t.Fatal(err)
		}
		protection := &upstream.Attempt{ID: uuid.NewString(), Identity: identity}
		for range 2 {
			if err := RecoverFallbackRouting(session, protection, "session", time.Now()); err != nil {
				t.Fatal(err)
			}
		}
		state, err := ReadLocalRoutingState(session.Item)
		if err != nil {
			t.Fatal(err)
		}
		if state.ConsecutiveFailures != 0 || state.OpenUntil != 0 || session.failureSnapshotInvalid || session.FailureGeneration != state.FailureGeneration {
			t.Fatalf("committed recovery was not followed: state=%+v session=%+v", state, session)
		}
	}
	var events int64
	if err := DB.Model(&upstream.Event{}).Where("kind = ? AND class = ?", "recovery", "fallback_recovered").Count(&events).Error; err != nil {
		t.Fatal(err)
	}
	if events != 3 {
		t.Fatalf("recovery events=%d, want 3", events)
	}
}

func TestLocalRoutingSessionSuccessDoesNotCloseCircuit(t *testing.T) {
	_, session := routingSessionFixture(t)
	policy, _ := localExecution(session.Item)
	for range policy.CircuitFailureThreshold {
		if err := RecordRoutingOutcome(session, false, 503, 0, time.Now(), true); err != nil {
			t.Fatal(err)
		}
	}
	if err := RecordRoutingOutcome(session, true, 200, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	state, err := ReadLocalRoutingState(session.Item)
	if err != nil {
		t.Fatal(err)
	}
	if state.OpenUntil == 0 || state.ConsecutiveFailures != policy.CircuitFailureThreshold || !session.failureSnapshotInvalid {
		t.Fatalf("ordinary success cleared an open circuit: state=%+v session=%+v", state, session)
	}
}

func TestFallbackRoutingSessionCannotRecoverExternalFailure(t *testing.T) {
	ch, session := routingSessionFixture(t)
	session.OrdinaryFallback, session.Sent = true, true
	BeginRoutingAttempt(session)
	external := *session
	if err := RecordRoutingOutcome(&external, false, 503, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := RecordRoutingOutcome(session, false, 503, 0, time.Now(), true); err != nil {
		t.Fatal(err)
	}
	protection := &upstream.Attempt{ID: uuid.NewString(), Identity: UpstreamIdentity(ch, "test-model", "upstream")}
	if err := RecoverFallbackRouting(session, protection, "session", time.Now()); err != nil {
		t.Fatal(err)
	}
	state, err := ReadLocalRoutingState(session.Item)
	if err != nil {
		t.Fatal(err)
	}
	if state.ConsecutiveFailures != 2 || !session.failureSnapshotInvalid {
		t.Fatalf("fallback cleared an external failure: state=%+v session=%+v", state, session)
	}
}
