package controller

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/relay/dto"
)

func TestSmartRetryChecksEveryCandidate(t *testing.T) {
	newTestDB(t)
	blocked := &model.Channel{Name: "blocked", Models: "review", Group: "default", Status: common.ChannelStatusEnabled, Priority: 100, Weight: 1}
	allowed := &model.Channel{Name: "allowed", Models: "review", Group: "default", Status: common.ChannelStatusEnabled, Priority: 1, Weight: 1}
	for _, ch := range []*model.Channel{blocked, allowed} {
		if err := model.InsertChannel(ch); err != nil {
			t.Fatal(err)
		}
	}
	checks := 0
	req := relayRequest{group: "default", modelName: "review", channelCheck: func(ch *model.Channel, group string) (bool, error) {
		checks++
		if group != "default" {
			t.Fatalf("unexpected group %q", group)
		}
		return ch.ID == allowed.ID, nil
	}}
	flow := newRelayFlow(context.Background(), httptest.NewRecorder(), req, dto.InboundOpenAI, "chat")
	defer flow.finish()
	ch, err := flow.pick()
	if err != nil || ch.ID != allowed.ID || checks != 2 || len(flow.trail) != 0 {
		t.Fatalf("pick=%v err=%v checks=%d trail=%v", ch, err, checks, flow.trail)
	}
	flow.begin(ch)
	if _, err := flow.pick(); !errors.Is(err, model.ErrNoChannel) {
		t.Fatalf("unexpected retry: %v", err)
	}
	if len(flow.trail) != 1 {
		t.Fatal("incompatible channel entered upstream trail")
	}
}

func TestSmartRetryConstraintFailureStopsSelection(t *testing.T) {
	newTestDB(t)
	if err := model.InsertChannel(&model.Channel{Name: "review", Models: "review", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}); err != nil {
		t.Fatal(err)
	}
	want := errors.New("metadata unavailable")
	req := relayRequest{group: "default", modelName: "review", channelCheck: func(*model.Channel, string) (bool, error) { return false, want }}
	flow := newRelayFlow(context.Background(), httptest.NewRecorder(), req, dto.InboundOpenAI, "chat")
	defer flow.finish()
	if _, err := flow.pick(); !errors.Is(err, want) {
		t.Fatalf("constraint failure lost: %v", err)
	}
	if len(flow.trail) != 0 {
		t.Fatal("constraint failure sent an upstream attempt")
	}
}
