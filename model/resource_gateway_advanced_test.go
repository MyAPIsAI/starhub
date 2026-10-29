package model

import (
	"context"
	"testing"
)

func TestAdvancedPublicationKeepsAliasPolicyAndRejectsConflictingTargets(t *testing.T) {
	ch, _, bindings := publicationFixture(t)
	ctx := context.Background()
	catalog, _ := ReadResourceCatalog(ctx)
	snapshot := *catalog.Snapshot
	snapshot.Sequence++
	snapshot.Instances[0].UpstreamModel = "provider-alpha"
	snapshot.Instances[0].Capabilities = []string{"text", "vision", "function_call", "responses"}
	catalog, err := SaveResourceSnapshot(ctx, catalog.Revision, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	name := "public-alpha"
	if _, err := SaveResourcePlacement(ctx, catalog.Revision, "resource-1", ResourcePlacement{ModelID: &name, GroupIDs: []string{"default"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	plan := publicationPlan(t, bindings)
	if !plan.Ready || plan.Profile != ResourceGatewayAdvancedProfile {
		t.Fatalf("advanced review: %+v", plan)
	}
	ack, err := PublishResourceGateway(ctx, publicationExpected(&plan.ResourcePublicationContext), bindings, plan.ReviewDigest, 7)
	if err != nil || ack.Profile != ResourceGatewayAdvancedProfile {
		t.Fatalf("publish: %+v %v", ack, err)
	}
	receipt, err := ReadResourcePublicationReceipt(ctx, plan.ReviewDigest)
	if err != nil || receipt.Routes[1].UpstreamModel != "provider-alpha" {
		t.Fatalf("receipt: %+v %v", receipt, err)
	}
	policy, managed, err := ReadResourceGatewayPolicy(ctx, ch.ID, "default", name)
	if err != nil || !managed || !policy.Allows("/v1/responses", []string{"function_call", "vision"}, 10, 20) || policy.UpstreamModel() != "provider-alpha" || policy.Allows("/v1/audio/speech", nil, 0, 0) {
		t.Fatal("advanced policy lost exact constraints")
	}
	if policy, managed, _ := ReadResourceGatewayPolicy(ctx, ch.ID, "vip", name); !managed || policy != nil {
		t.Fatal("alias crossed group boundary")
	}
	if candidate, err := GetSatisfiedChannel("default", name); err != nil || candidate.ID != ch.ID {
		t.Fatalf("advanced SQL eligibility: %+v %v", candidate, err)
	}
	// A new inventory identity revokes the old grant before a fresh publication.
	catalog, _ = ReadResourceCatalog(ctx)
	snapshot.Sequence++
	duplicate := snapshot.Instances[0]
	duplicate.ID = "conflict"
	duplicate.UpstreamModel = "other-provider-model"
	snapshot.Instances = append(snapshot.Instances, duplicate)
	catalog, err = SaveResourceSnapshot(ctx, catalog.Revision, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = SaveResourcePlacement(ctx, catalog.Revision, duplicate.ID, ResourcePlacement{ModelID: &name, GroupIDs: []string{"default"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	plan = publicationPlan(t, bindings)
	if plan.Ready || !publicationIssue(plan, "incompatible_resource_constraints") {
		t.Fatal("conflicting upstream mappings were merged")
	}
	if err := UpdateChannelStatus(ch.ID, 1); err != nil {
		t.Fatal(err)
	}
	if policy, managed, _ := ReadResourceGatewayPolicy(ctx, ch.ID, "default", name); !managed || policy != nil {
		t.Fatal("ordinary enable revived revoked policy")
	}
}

func TestAdvancedNativeProtocolAndEndpointCapabilities(t *testing.T) {
	for _, protocol := range []string{"OpenAI", "Anthropic", "Gemini"} {
		route := ResourceGatewayRoute{UpstreamModel: "provider-model", Protocols: `["` + protocol + `"]`, Capabilities: `["text","function_call","vision","responses","image","video","audio","embedding","rerank"]`}
		if protocol != "OpenAI" {
			route.Capabilities = `["text","function_call","vision"]`
		}
		policy := resourceGatewayPolicyForRoute(route)
		if policy == nil {
			t.Fatal(protocol)
		}
		if got := policy.Allows("/v1beta/models", []string{"gemini", "function_call", "vision"}, 1, 1); got != (protocol == "Gemini") {
			t.Fatal("native Gemini protocol mismatch")
		}
		for _, path := range []string{"/v1/responses", "/v1/images/generations", "/v1/audio/speech", "/v1/video/generations", "/v1/embeddings", "/v1/rerank"} {
			if got := policy.Allows(path, nil, 1, 1); got != (protocol == "OpenAI") {
				t.Fatalf("%s %s grant=%v", protocol, path, got)
			}
		}
	}
	for _, route := range []ResourceGatewayRoute{
		{UpstreamModel: "model", Protocols: `["OpenAI"]`, Capabilities: `["vision"]`},
		{UpstreamModel: "model", Protocols: `["Anthropic"]`, Capabilities: `["video"]`},
		{UpstreamModel: "model", Protocols: `["Gemini"]`, Capabilities: `["text","audio"]`},
	} {
		if resourceGatewayPolicyForRoute(route) != nil {
			t.Fatal("resource without supported protocol endpoints was accepted")
		}
	}
	for _, caps := range []string{`["text","unknown"]`, `["text","text"]`, `null`, `{}`, `[]`} {
		if resourceGatewayPolicyForRoute(ResourceGatewayRoute{UpstreamModel: "model", Protocols: `["OpenAI"]`, Capabilities: caps}) != nil {
			t.Fatalf("invalid capabilities accepted %s", caps)
		}
	}
}
