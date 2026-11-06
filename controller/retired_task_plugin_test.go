package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"starhub/middleware"
	"starhub/model"
	"starhub/relay/dto"
)

func TestPublicVideoHandlerRejectsLegacyPluginWithoutCharging(t *testing.T) {
	var calls atomic.Int64
	f := newPluginPollingFixture(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }, false)
	before, _ := model.GetUserByID(f.user.ID)
	keyBefore, _ := model.GetTokenByID(f.token.ID)
	var tasksBefore int64
	model.DB.Model(&model.Task{}).Count(&tasksBefore)
	r := httptest.NewRequest("POST", "/v1/video/generations", strings.NewReader(`{"model":"sora-2","prompt":"A cat","seconds":8}`))
	r.Header.Set("Authorization", "Bearer "+f.token.Key)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(http.HandlerFunc(RelayTaskSubmitVideo))).ServeHTTP(w, r)
	if w.Code < 400 {
		t.Fatalf("public plugin admission still available: %d", w.Code)
	}
	after, _ := model.GetUserByID(f.user.ID)
	keyAfter, _ := model.GetTokenByID(f.token.ID)
	var tasksAfter int64
	model.DB.Model(&model.Task{}).Count(&tasksAfter)
	if calls.Load() != 0 || after.Quota != before.Quota || keyAfter.RemainQuota != keyBefore.RemainQuota || tasksAfter != tasksBefore {
		t.Fatal("retired plugin admission submitted or reserved a new task")
	}
}
