package relay

import (
	"bytes"
	"log"
	"path/filepath"
	"strings"
	"testing"

	"starhub/model"
	"starhub/setting"
)

func TestTokenPricingFallbackIsAudited(t *testing.T) {
	setting.ResetRatiosToDefaults()
	resetPricingForTests()
	t.Cleanup(setting.ResetRatiosToDefaults)
	t.Cleanup(resetPricingForTests)
	var output bytes.Buffer
	oldWriter := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(oldWriter) })
	if got := ComputeQuota("unpriced-model", "default", 10, 10); got != 25 {
		t.Fatalf("fallback quota=%d", got)
	}
	for _, marker := range []string{"[billing_ratio_fallback]", `model="unpriced-model"`, "option=model_ratio", "option=completion_ratio"} {
		if !strings.Contains(output.String(), marker) {
			t.Fatalf("missing %q in audit: %s", marker, output.String())
		}
	}
	output.Reset()
	_ = ComputeQuota("claude-sonnet-4-6", "default", 10, 10)
	if output.Len() != 0 {
		t.Fatalf("known model produced a fallback warning: %s", output.String())
	}
	if err := setting.UpdatePricing(setting.OptionKeyModelPrice, `{"per-call-model":0.01}`); err != nil {
		t.Fatal(err)
	}
	_ = ComputeQuota("per-call-model", "default", 10, 10)
	if output.Len() != 0 {
		t.Fatalf("per-call pricing must not warn about unused token ratios: %s", output.String())
	}
	if err := setting.UpdateRatio(setting.OptionKeyModelRatio, `{"partial-model":2}`); err != nil {
		t.Fatal(err)
	}
	_ = ComputeQuota("partial-model", "default", 10, 10)
	if strings.Contains(output.String(), "option=model_ratio") || !strings.Contains(output.String(), "option=completion_ratio") {
		t.Fatalf("partial configuration warning is wrong: %s", output.String())
	}
}

func TestAffiliateRebateUsesCollectedQuota(t *testing.T) {
	if err := model.InitDB(filepath.Join(t.TempDir(), "billing.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = model.CloseDB() })
	setting.ResetAffiliateToDefaults()
	t.Cleanup(setting.ResetAffiliateToDefaults)
	if err := setting.UpdateAffiliate(setting.OptionKeyAffRebateRatio, "0.1"); err != nil {
		t.Fatal(err)
	}
	inviter := &model.User{Username: "inviter", AffCode: "inviter"}
	if err := model.CreateUser(inviter); err != nil {
		t.Fatal(err)
	}
	for _, explicit := range []bool{false, true} {
		name := "usage"
		if explicit {
			name = "explicit"
		}
		t.Run(name, func(t *testing.T) {
			user := &model.User{Username: name, AffCode: name, Quota: 100, InviterID: inviter.ID}
			if err := model.CreateUser(user); err != nil {
				t.Fatal(err)
			}
			token := &model.Token{UserID: user.ID, RemainQuota: 100}
			if err := model.InsertToken(token); err != nil {
				t.Fatal(err)
			}
			if err := model.ReserveQuota(user.ID, token.ID, false, 80); err != nil {
				t.Fatal(err)
			}
			before, err := model.GetUserByID(inviter.ID)
			if err != nil {
				t.Fatal(err)
			}
		if explicit {
			err = PostConsumeQuotaAmount(user.ID, token.ID, "", false, 0, "m", 130, 80, "", 0)
		} else {
			pd := &PriceData{GroupRatio: 1, OtherRatio: 1, ModelPriceUSD: 0.00026}
			err = PostConsumeQuotaWithPrice(pd, user.ID, token.ID, "", false, 0, "m", &Usage{}, 80, "", 0)
		}
			if err != nil {
				t.Fatal(err)
			}
			after, err := model.GetUserByID(inviter.ID)
			if err != nil || after.Quota-before.Quota != 10 {
				t.Fatalf("rebate must be 10 on collected 100, before=%+v after=%+v err=%v", before, after, err)
			}
		})
	}
}
