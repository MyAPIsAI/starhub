package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"starhub/model"
)

// This local fixture compares scheduler work, not production relay capacity.
func BenchmarkReconciliationHistoryScheduling(b *testing.B) {
	if err := model.InitDB(filepath.Join(b.TempDir(), "reconciliation.db")); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = model.CloseDB() })
	u := &model.User{Username: "history-benchmark", AffCode: "history-benchmark"}
	if err := model.CreateUser(u); err != nil {
		b.Fatal(err)
	}
	for i := 2; i < 102; i++ {
		at := time.Now().UTC().AddDate(0, 0, -i)
		if err := model.InsertLog(&model.Log{UserID: u.ID, Status: 2, CreatedAt: at.Unix()}); err != nil {
			b.Fatal(err)
		}
	}
	RunDailyReconciliationOnce(context.Background())
	if err := model.DB.Model(&model.ReconciliationWork{}).Where("user_id = ?", u.ID).UpdateColumn("dirty", false).Error; err != nil {
		b.Fatal(err)
	}
	for _, mode := range []string{"full_history", "clean_minute", "one_changed_day"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if mode == "full_history" {
					RunDailyReconciliationOnce(context.Background())
					continue
				}
				if mode == "one_changed_day" {
					var row model.ReconciliationWork
					if err := model.DB.Where("user_id = ?", u.ID).Order("eligible_at").First(&row).Error; err != nil {
						b.Fatal(err)
					}
					if err := model.DB.Model(&row).UpdateColumn("dirty", true).Error; err != nil {
						b.Fatal(err)
					}
				}
				RunChangedDayReconciliationOnce(context.Background())
			}
		})
	}
}
