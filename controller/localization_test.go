package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"starhub/common"
	"starhub/model"
	"strings"
	"testing"
	"time"
)

func TestDisabledArabicUsesEnglishForExistingAccountsAndMail(t *testing.T) {
	for _, defaultLanguage := range []string{"en", "zh", "ar"} {
		t.Run(defaultLanguage, func(t *testing.T) {
			newTestDB(t)
			owner := newUser(t, "arabic-owner", common.RoleUser, 0)
			if err := model.DB.Model(owner).Updates(map[string]any{"preferred_language": "ar", "email": "owner@example.test"}).Error; err != nil {
				t.Fatal(err)
			}
			profile := model.DefaultDeploymentProfile()
			profile.DefaultLanguage = defaultLanguage
			profile.Features["arabic"] = false
			version, err := model.SaveDeploymentProfile(t.Context(), profile, 0, owner.ID, "127.0.0.1")
			if err != nil {
				t.Fatal(err)
			}
			response := call(GetLocalizationPolicy, testRequest{target: "/api/localization"})
			var body struct {
				Data model.LocalizationPolicy `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != 200 || slices.Contains(body.Data.Languages, "ar") || !slices.Contains(body.Data.Languages, "en") {
				t.Fatalf("invalid disabled policy: %s / %v", response.Body, err)
			}
			language, err := model.EffectiveAccountLanguage(t.Context(), "ar")
			if err != nil || language != "en" {
				t.Fatalf("disabled Arabic resolved to %q: %v", language, err)
			}
			response = call(PutAccountLanguage, testRequest{method: http.MethodPut, target: "/api/user/language", userID: owner.ID, body: fmt.Sprintf(`{"expected_user_id":%d,"language":"ar"}`, owner.ID)})
			if response.Code != 400 {
				t.Fatal("disabled Arabic could be selected", response.Body)
			}
			user, _ := model.GetUserByID(owner.ID)
			if user.PreferredLanguage != "ar" {
				t.Fatal("disabling Arabic erased the saved preference")
			}
			if err := model.UpdateOwnedProfile(user, "password", user.Password); err != nil {
				t.Fatal(err)
			}
			var mail model.TransactionalMail
			if err := model.DB.First(&mail).Error; err != nil || mail.Language != "en" {
				t.Fatalf("mail did not fall back to English: %+v / %v", mail, err)
			}
			profile.Features["arabic"] = true
			if _, err := model.SaveDeploymentProfile(t.Context(), profile, version.Revision, owner.ID, "127.0.0.1"); err != nil {
				t.Fatal(err)
			}
			language, err = model.EffectiveAccountLanguage(t.Context(), user.PreferredLanguage)
			if err != nil || language != "ar" {
				t.Fatal("reenabling Arabic lost the saved language", err)
			}
		})
	}
}

func TestAccountLanguagePersistsAndLocalizesTransactionalMail(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "language-owner", common.RoleUser, 0)
	other := newUser(t, "language-other", common.RoleUser, 0)
	for _, input := range []struct {
		id       int64
		language string
		want     int
	}{{other.ID, "zh", 409}, {owner.ID, "ko", 400}, {owner.ID, "zh", 200}} {
		response := call(PutAccountLanguage, testRequest{method: http.MethodPut, target: "/api/user/language", userID: owner.ID, body: fmt.Sprintf(`{"expected_user_id":%d,"language":%q}`, input.id, input.language)})
		if response.Code != input.want {
			t.Fatalf("language response %d: %s", response.Code, response.Body.String())
		}
	}
	response := call(GetSelf, testRequest{target: "/api/user/self", userID: owner.ID})
	if !strings.Contains(response.Body.String(), `"preferred_language":"zh"`) {
		t.Fatal("restored account lost language")
	}
	if err := model.DB.Model(owner).Update("email", "language@example.test").Error; err != nil {
		t.Fatal(err)
	}
	user, _ := model.GetUserByID(owner.ID)
	if err := model.UpdateOwnedProfile(user, "password", user.Password); err != nil {
		t.Fatal(err)
	}
	var messages []model.TransactionalMail
	if err := model.DB.Find(&messages).Error; err != nil || len(messages) != 1 || messages[0].Language != "zh" {
		t.Fatalf("mail language not frozen: %+v %v", messages, err)
	}
	attempts := 0
	if err := model.DeliverTransactionalMail(t.Context(), time.Now(), func(_ context.Context, row model.TransactionalMail) error {
		attempts++
		subject, body, err := model.RenderTransactionalMail(row.Kind, row.Language)
		if err != nil || !strings.Contains(subject, "密码") || !strings.Contains(body, "账号") {
			t.Fatal("wrong transaction translation")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Fatal("mail not dispatched")
	}
}

func TestTransactionalMailRetryKeepsFrozenLanguageAndDoesNotResendSuccess(t *testing.T) {
	newTestDB(t)
	row := model.TransactionalMail{UserID: 1, Recipient: "recipient@example.test", Language: "ar", Kind: "password_changed", Status: "pending"}
	if err := model.DB.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	attempts := 0
	send := func(_ context.Context, value model.TransactionalMail) error {
		attempts++
		if value.ID != row.ID || value.Language != "ar" {
			t.Fatal("retry changed the committed message")
		}
		if attempts == 1 {
			return errors.New("temporary failure")
		}
		return nil
	}
	for _, offset := range []time.Duration{0, 30 * time.Second, 2 * time.Minute, 3 * time.Minute} {
		if err := model.DeliverTransactionalMail(t.Context(), now.Add(offset), send); err != nil {
			t.Fatal(err)
		}
		if offset == 0 {
			var file string
			if err := model.DB.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&file).Error; err != nil {
				t.Fatal(err)
			}
			if err := model.CloseDB(); err != nil {
				t.Fatal(err)
			}
			if err := model.InitDB(file); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := model.DB.First(&row, row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || row.Attempts != 2 || row.Status != "sent" || row.SentAt == 0 {
		t.Fatalf("retry state: %+v / %d", row, attempts)
	}
}

func TestTransactionalMailQueueFailureRollsBackSecurityChange(t *testing.T) {
	newTestDB(t)
	owner := newUser(t, "mail-rollback", common.RoleUser, 0)
	if err := model.DB.Model(owner).Update("email", "owner@example.test").Error; err != nil {
		t.Fatal(err)
	}
	owner, _ = model.GetUserByID(owner.ID)
	if err := model.DB.Migrator().DropTable(&model.TransactionalMail{}); err != nil {
		t.Fatal(err)
	}
	if err := model.UpdateOwnedProfile(owner, "email", "changed@example.test"); err == nil {
		t.Fatal("expected queue failure")
	}
	current, err := model.GetUserByID(owner.ID)
	if err != nil || current.Email != owner.Email {
		t.Fatal("security change escaped rollback")
	}
	var count int64
	if err := model.DB.Model(&model.UserSecurityEvent{}).Where("user_id = ?", owner.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("security event escaped rollback")
	}
}
