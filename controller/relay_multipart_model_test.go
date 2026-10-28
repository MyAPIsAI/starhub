package controller

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay/channel"
	"starhub/relay/dto"
	"starhub/setting"
)

func TestRelayMultipartModelReachesUpstream(t *testing.T) {
	for _, tc := range []struct {
		name, path, actual, requested, fileField, response string
		handler                                            http.HandlerFunc
		retry                                              bool
		charge                                             int64
	}{
		{"transcription", "/v1/audio/transcriptions", "whisper-1", "myapis-auto", "file", `{"text":"review","duration":2}`, RelaySTT, false, 5000},
		{"translation", "/v1/audio/translations", "whisper-1", "myapis-auto", "file", `{"text":"review","duration":2}`, RelaySTTTranslations, false, 5000},
		{"image edit", "/v1/images/edits", "gpt-image-1", "myapis-auto", "image", `{"data":[{"b64_json":"QUJD"}]}`, RelayImageEdits, false, 20000},
		{"explicit transcription", "/v1/audio/transcriptions", "whisper-1", "whisper-1", "file", `{"text":"review","duration":2}`, RelaySTT, false, 5000},
		{"transcription retry", "/v1/audio/transcriptions", "whisper-1", "myapis-auto", "file", `{"text":"review","duration":2}`, RelaySTT, true, 5000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newTestDB(t)
			if err := setting.UpdatePricing(setting.OptionKeyModelPrice, `{"whisper-1":0.01,"gpt-image-1":0.04}`); err != nil {
				t.Fatal(err)
			}
			type receipt struct {
				body        []byte
				contentType string
				path        string
				length      int64
			}
			captured := make(chan receipt, common.RetryTimes+1)
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "read failed", http.StatusBadRequest)
					return
				}
				captured <- receipt{body, r.Header.Get("Content-Type"), r.URL.Path, r.ContentLength}
				w.Header().Set("Content-Type", "application/json")
				if calls.Add(1) == 1 && tc.retry {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = io.WriteString(w, `{"error":{"message":"retry review"}}`)
					return
				}
				_, _ = io.WriteString(w, tc.response)
			}))
			defer upstream.Close()
			channels := 1
			if tc.retry {
				channels = 2
			}
			for i := 0; i < channels; i++ {
				if err := model.InsertChannel(&model.Channel{Name: "multipart-review", Type: channel.ChannelTypeOpenAI, Key: "review-key", BaseURL: upstream.URL, Models: tc.actual, Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}); err != nil {
					t.Fatal(err)
				}
			}
			user := newUser(t, "multipart-review", common.RoleUser, 1000000)
			token := mustToken(t, user.ID, 1000000)
			fileBytes := append([]byte{0, 255, 13, 10}, bytes.Repeat([]byte("myapis-auto"), 1000)...)
			body, contentType, err := common.BuildMultipartForm(&common.MultipartForm{
				Values: map[string]string{"model": tc.requested, "prompt": "keep myapis-auto", "n": "1", "response_format": "json"},
				Files:  []common.MultipartFile{{FieldName: tc.fileField, FileName: "review.bin", ContentType: "application/octet-stream", Data: fileBytes}},
			})
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(body))
			r.Header.Set("Content-Type", contentType)
			r.Header.Set("Authorization", "Bearer "+token.Key)
			rec := httptest.NewRecorder()
			middleware.TokenAuth(middleware.Distribute(dto.InboundOpenAI)(tc.handler)).ServeHTTP(rec, r)
			if rec.Code != http.StatusOK || calls.Load() != int32(channels) {
				t.Fatalf("status=%d calls=%d, want 200/%d: %s", rec.Code, calls.Load(), channels, rec.Body)
			}
			var replay []byte
			for i := 0; i < channels; i++ {
				sent := <-captured
				form, err := common.ParseMultipartForm(sent.contentType, sent.body)
				if err != nil {
					t.Fatal(err)
				}
				file := form.File(tc.fileField)
				if sent.path != tc.path || sent.contentType != contentType || sent.length != int64(len(sent.body)) || form.Value("model") != tc.actual || form.Value("prompt") != "keep myapis-auto" || form.Value("response_format") != "json" || file == nil || file.FileName != "review.bin" || !bytes.Equal(file.Data, fileBytes) {
					t.Fatal("upstream multipart metadata, actual model or upload changed")
				}
				if i > 0 && !bytes.Equal(replay, sent.body) {
					t.Fatal("retry did not replay the same routed multipart body")
				}
				if tc.requested == tc.actual && !bytes.Equal(sent.body, body) {
					t.Fatal("explicit-model upload changed")
				}
				replay = sent.body
			}
			entries, total, err := model.ListLogs(user.ID, 1, 10, tc.actual)
			if err != nil || total != 1 || len(entries) != 1 || entries[0].RequestedModel != tc.requested || entries[0].Model != tc.actual || entries[0].Quota != tc.charge || entries[0].Status != 1 {
				t.Fatalf("model attribution or single settlement changed: %+v, total=%d err=%v", entries, total, err)
			}
			afterUser, err := model.GetUserByID(user.ID)
			if err != nil {
				t.Fatal(err)
			}
			afterToken, err := model.GetTokenByID(token.ID)
			if err != nil {
				t.Fatal(err)
			}
			if afterUser.Quota != user.Quota-tc.charge || afterUser.UsedQuota != user.UsedQuota+tc.charge || afterToken.RemainQuota != token.RemainQuota-tc.charge || afterToken.UsedQuota != token.UsedQuota+tc.charge {
				t.Fatal("multipart request was not charged exactly once")
			}
		})
	}
}
