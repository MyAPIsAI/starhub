package controller

import (
	"fmt"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestOwnedTokenChoiceDisplayMetadataDoesNotExpandAccountScope(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "choice-owner", common.RoleUser, 1000)
	other := newUser(t, "choice-other", common.RoleUser, 1000)
	if _, err := model.SaveOption("group_ratio", `{"default":1,"private":1}`, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := model.UpsertServiceGroup(&model.ServiceGroupConfig{GroupID: "private", Visibility: "custom", Enabled: true, AccountIDs: itoa(other.ID)}); err != nil {
		t.Fatal(err)
	}
	for _, channel := range []*model.Channel{
		{Name: "public choices", Type: 1, Status: 1, Key: "synthetic-choice-key", Models: "gpt-example,text-embedding-example", Group: "default"},
		{Name: "private choices", Type: 1, Status: 1, Key: "synthetic-private-key", Models: "private-model", Group: "private"},
	} {
		if err := model.InsertChannel(channel); err != nil {
			t.Fatal(err)
		}
	}
	for _, meta := range []*model.ModelMeta{
		{Model: "gpt-example", Vendor: "test-vendor", VendorDisplay: "Example Vendor", Tags: "vision,function_call"},
		{Model: "text-embedding-example", Vendor: "test-vendor", VendorDisplay: "Example Vendor", Tags: "embedding"},
		{Model: "private-model", Vendor: "private-vendor", VendorDisplay: "Private Vendor"},
	} {
		if err := model.UpsertModelMeta(meta); err != nil {
			t.Fatal(err)
		}
	}
	response := call(GetOwnedTokenChoices, testRequest{method: "GET", target: fmt.Sprintf("/api/token/owned-choices?expected_user_id=%d", owner.ID), userID: owner.ID, role: owner.Role})
	var result struct {
		Revision string                   `json:"revision"`
		Choices  []model.OwnedTokenChoice `json:"choices"`
	}
	decodeData(t, response, &result)
	if len(result.Choices) != 2 {
		t.Fatalf("unexpected owner choices: %+v", result.Choices)
	}
	for _, choice := range result.Choices {
		if choice.Vendor != "test-vendor" || choice.VendorDisplay != "Example Vendor" || len(choice.Tags) == 0 {
			t.Fatalf("missing display metadata: %+v", choice)
		}
		if choice.SmartEligible != (choice.Model == "gpt-example") {
			t.Fatalf("non-text smart candidate eligibility changed: %+v", choice)
		}
	}
	if strings.Contains(response.Body.String(), "private-model") || strings.Contains(response.Body.String(), "Private Vendor") || strings.Contains(response.Body.String(), "synthetic-") || strings.Contains(response.Body.String(), "official_input") {
		t.Fatal("display metadata exposed private catalog or credential fields")
	}
	state, err := model.ReadOwnedTokenChoices(owner.ID)
	if err != nil || result.Revision != state.Revision {
		t.Fatal("display metadata changed the mutation guard")
	}
}
