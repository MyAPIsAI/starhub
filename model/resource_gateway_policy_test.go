package model

import (
	"context"
	"testing"

	"starhub/common"
)

func TestResourceGatewayPolicyBoundsAndImmutableSnapshot(t *testing.T) {
	newTestDB(t)
	ch := addChannel(t, "managed", "default", "alpha", 0, 1, common.ChannelStatusEnabled)
	managedGatewayFixture(t, ch, true, ResourceGatewayRoute{Model: "alpha", Group: "default"})
	if err := DB.Model(&ResourceGatewayRoute{}).Where("channel_id = ?", ch.ID).Updates(map[string]any{"context_length": 1000, "capabilities": `["text"]`}).Error; err != nil {
		t.Fatal(err)
	}
	policy, managed, err := ReadResourceGatewayPolicy(context.Background(), ch.ID, "default", "alpha")
	if err != nil || !managed || policy == nil {
		t.Fatalf("policy unavailable: %v %v", managed, err)
	}
	if !policy.Allows("/v1/chat/completions", []string{"text"}, 900, 100) || !policy.Allows("/v1/messages", nil, 1, 1) {
		t.Fatal("basic text boundary rejected")
	}
	for _, test := range []struct {
		path          string
		caps          []string
		input, output int64
	}{{"/v1/chat/completions", nil, 900, 101}, {"/v1/chat/completions", nil, -1, 10}, {"/v1/chat/completions", nil, 1, maxResourceRevision + 1}, {"/v1/images/generations", nil, 1, 1}, {"/v1/responses", nil, 1, 1}, {"/v1/realtime", nil, 0, 0}, {"/v1/chat/completions", []string{"code"}, 1, 1}, {"/v1/chat/completions", []string{"vision"}, 1, 1}, {"/v1/chat/completions", []string{"function_call"}, 1, 1}} {
		if policy.Allows(test.path, test.caps, test.input, test.output) {
			t.Fatalf("unsupported policy request accepted: %+v", test)
		}
	}
	if err := DB.Model(&ResourceGatewayRoute{}).Where("channel_id = ?", ch.ID).Updates(map[string]any{"context_length": 10, "capabilities": `["code","text"]`}).Error; err != nil {
		t.Fatal(err)
	}
	next, managed, err := ReadResourceGatewayPolicy(context.Background(), ch.ID, "default", "alpha")
	if err != nil || !managed || !next.Allows("/v1/chat/completions", []string{"code"}, 1, 1) || next.Allows("/v1/chat/completions", nil, 900, 100) || !policy.Allows("/v1/chat/completions", nil, 900, 100) {
		t.Fatalf("policy snapshot mutated: %v", err)
	}
	if _, managed, err := ReadResourceGatewayPolicy(context.Background(), ch.ID+1, "default", "alpha"); err != nil || managed {
		t.Fatalf("unmanaged channel was restricted: %v", err)
	}
}

func TestResourceGatewayUnknownPoliciesAndInvalidStoredLimitsFailClosed(t *testing.T) {
	for _, invalid := range []string{"profile", "empty-caps", "unknown-caps", "noncanonical-caps", "negative-context", "unsafe-context", "channel-type"} {
		t.Run(invalid, func(t *testing.T) {
			newTestDB(t)
			ch := addChannel(t, "managed", "default", "alpha", 0, 1, common.ChannelStatusEnabled)
			managedGatewayFixture(t, ch, true, ResourceGatewayRoute{Model: "alpha", Group: "default"})
			var err error
			switch invalid {
			case "profile":
				err = DB.Model(&ResourceGatewayManagedChannel{}).Where("channel_id = ?", ch.ID).Update("profile", "unknown").Error
			case "empty-caps":
				err = DB.Model(&ResourceGatewayRoute{}).Where("channel_id = ?", ch.ID).Update("capabilities", `[]`).Error
			case "unknown-caps":
				err = DB.Model(&ResourceGatewayRoute{}).Where("channel_id = ?", ch.ID).Update("capabilities", `["vision"]`).Error
			case "noncanonical-caps":
				err = DB.Model(&ResourceGatewayRoute{}).Where("channel_id = ?", ch.ID).Update("capabilities", `["text", "code"]`).Error
			case "negative-context":
				err = DB.Model(&ResourceGatewayRoute{}).Where("channel_id = ?", ch.ID).Update("context_length", -1).Error
			case "unsafe-context":
				err = DB.Model(&ResourceGatewayRoute{}).Where("channel_id = ?", ch.ID).Update("context_length", maxResourceRevision+1).Error
			case "channel-type":
				err = DB.Model(&Channel{}).Where("id = ?", ch.ID).Update("type", 3).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			policy, managed, err := ReadResourceGatewayPolicy(context.Background(), ch.ID, "default", "alpha")
			if err != nil || !managed || policy.Allows("/v1/chat/completions", nil, 1, 1) {
				t.Fatalf("invalid %s allowed: %v", invalid, err)
			}
			if names, err := ListModelsByGroup("default"); err != nil || len(names) != 0 {
				t.Fatalf("invalid policy advertised: %v %v", names, err)
			}
		})
	}
}

func TestResourceGatewayCredentialIdentityChangesRequireRepublication(t *testing.T) {
	for _, field := range []string{"key", "base-url", "type"} {
		t.Run(field, func(t *testing.T) {
			newTestDB(t)
			ch := addChannel(t, "managed", "default,vip", "alpha,beta", 0, 1, common.ChannelStatusEnabled)
			managedGatewayFixture(t, ch, true, ResourceGatewayRoute{Model: "alpha", Group: "default"}, ResourceGatewayRoute{Model: "beta", Group: "vip"})
			original := *ch
			switch field {
			case "key":
				ch.Key = "rotated-synthetic"
			case "base-url":
				ch.BaseURL = "https://changed.invalid"
			case "type":
				ch.Type = 3
			}
			if err := UpdateChannel(ch); err != nil {
				t.Fatal(err)
			}
			assertGatewayPairs(t, ch.ID, false)
			if err := UpdateChannel(&original); err != nil {
				t.Fatal(err)
			}
			assertGatewayPairs(t, ch.ID, false)
		})
	}
}
