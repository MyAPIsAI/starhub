//go:build ignore

package main

import (
	"fmt"
	"os"
	"starhub/common"
	"starhub/model"
	"time"
)

func main() {
	if err := model.InitDB(os.Args[1]); err != nil {
		panic(err)
	}
	defer model.CloseDB()
	connection, err := model.DB.DB()
	if err != nil {
		panic(err)
	}
	connection.SetMaxOpenConns(1)
	if err := model.DB.Exec("PRAGMA temp_store = MEMORY").Error; err != nil {
		panic(err)
	}
	loc, _ := time.LoadLocation("Asia/Dubai")
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	profile := model.DefaultDeploymentProfile()
	profile.RegionID = "middle-east"
	profile.Timezone = loc.String()
	if err := model.DB.Create(&model.DeploymentProfileVersion{Revision: 1, Profile: profile}).Error; err != nil {
		panic(err)
	}
	groups := []string{"Ultra", "Pro", "Standard", "Lite"}
	for _, group := range groups {
		if err := model.UpsertServiceGroup(&model.ServiceGroupConfig{GroupID: group, Name: group, Enabled: true}); err != nil {
			panic(err)
		}
	}
	for i := 0; i < 25; i++ {
		user := &model.User{Username: fmt.Sprintf("sample_customer_%02d", i+1), AffCode: fmt.Sprintf("dashboard-browser-%d", i), Role: 1, Status: 1, CreatedAt: today.AddDate(0, 0, -i).Unix()}
		if err := model.CreateUser(user); err != nil {
			panic(err)
		}
		m := fmt.Sprintf("sample-model-%02d", i+1)
		if err := model.DB.Create(&model.StandardModel{Model: m, Name: fmt.Sprintf("Example Model %02d", i+1)}).Error; err != nil {
			panic(err)
		}
		if i < 8 {
			if _, err := model.StaffAdjustBalance(0, user.ID, int64(10000-i*500)*1_000_000_000, "offline_payment", "Synthetic browser receipt", fmt.Sprint(i)); err != nil {
				panic(err)
			}
		}
		logs := []model.Log{}
		for day := 0; day < 7; day++ {
			for j := 0; j < 8; j++ {
				status := 1
				if i == 24 && j == 7 {
					status = 2
				}
				quota := int64(25-i) * common.QuotaPerUSD * int64(day+1) / 3
				if status != 1 {
					quota = 0
				}
				logs = append(logs, model.Log{UserID: user.ID, Model: m, Vendor: []string{"Anthropic", "OpenAI", "Google", "DeepSeek", "Kimi", "Example A", "Example B", "Example C"}[i%8], ServiceGroup: groups[(i+j)%4], CallType: []string{"chat", "images", "video", "embeddings", "audio"}[i%5], Status: status, Quota: quota, CreatedAt: today.AddDate(0, 0, -day).Add(time.Duration(j) * time.Minute).Unix()})
			}
		}
		if err := model.DB.CreateInBatches(&logs, 1).Error; err != nil {
			panic(err)
		}
	}
}
