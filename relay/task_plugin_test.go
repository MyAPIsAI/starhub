package relay

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"starhub/model"
	"starhub/pkg/taskplugin"
)

func TestPluginPricingStrictAndSnapshot(t *testing.T) {
	u := taskplugin.Usage{Seconds: 8, N: 2}
	pd := &PriceData{HasModelPrice: true, ModelPriceUSD: 0.01, GroupRatio: 1, GroupGroupRatio: 1, OtherRatio: 1}
	if q, err := PluginTaskQuota(pd, u, "succeeded"); err != nil || q != 80000 {
		t.Fatalf("unit pricing: %d %v", q, err)
	}
	pd.PriceExpr = "seconds * n * 0.02"
	if q, err := PluginTaskQuota(pd, u, "succeeded"); err != nil || q != 160000 {
		t.Fatalf("expression: %d %v", q, err)
	}
	for _, expr := range []string{"-1", "1 / 0", "bad("} {
		pd.PriceExpr = expr
		if _, err := PluginTaskQuota(pd, u, "succeeded"); err == nil {
			t.Fatalf("invalid expression accepted: %s", expr)
		}
	}
	pd.PriceExpr = ""
	pd.HasModelPrice = false
	if _, err := PluginTaskQuota(pd, u, "succeeded"); err == nil {
		t.Fatal("missing price accepted")
	}
	if q, err := PluginTaskQuota(pd, u, "failed"); err != nil || q != 0 {
		t.Fatalf("refund: %d %v", q, err)
	}
	if _, err := PluginTaskQuota(pd, taskplugin.Usage{Seconds: 8, N: 1.5}, "succeeded"); err == nil {
		t.Fatal("fractional count accepted")
	}
}

func TestPluginLoadAfterStorageReopenAndCacheReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.db")
	if err := model.InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = model.CloseDB(); taskPluginCache = sync.Map{} })
	source, err := os.ReadFile("../plugins/tasks/openai-video.js")
	if err != nil {
		t.Fatal(err)
	}
	plugin, err := taskplugin.Compile(context.Background(), string(source))
	if err != nil {
		t.Fatal(err)
	}
	stored, err := model.SaveTaskPlugin(string(source), plugin.Meta)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTaskPlugin(context.Background(), stored.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := model.SetTaskPluginEnabled(stored.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := model.CloseDB(); err != nil {
		t.Fatal(err)
	}
	taskPluginCache = sync.Map{}
	if err := model.InitDB(path); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTaskPlugin(context.Background(), stored.ID, false); err != nil {
		t.Fatalf("old disabled version unavailable after restart: %v", err)
	}
	if _, err := LoadTaskPlugin(context.Background(), stored.ID, true); err == nil {
		t.Fatal("disabled version accepted new task after restart")
	}
}
