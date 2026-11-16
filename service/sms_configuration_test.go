package service_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/controller"
	"starhub/internal/router"
	"starhub/middleware"
	"starhub/model"
	"starhub/service"
	"starhub/setting"
)

type smsTransport func(*http.Request) (*http.Response, error)

func (f smsTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSMSConfigurationAuthorizationPersistenceAndSending(t *testing.T) {
	t.Setenv("STARHUB_ENV", "development")
	t.Setenv("STARHUB_SMS_BRIDGE_URL", "")
	t.Setenv("STARHUB_SMS_BRIDGE_TOKEN", "")
	t.Setenv("STARHUB_CREDENTIAL_KEYS", `{"fixture":"`+strings.Repeat("11", 32)+`"}`)
	t.Setenv("STARHUB_CREDENTIAL_ACTIVE_KEY", "fixture")
	setting.ResetAllToDefaults()
	middleware.ResetRateLimits()
	file := filepath.Join(t.TempDir(), "sms.db")
	if err := model.InitDB(file); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		service.SetSMSProvider(nil)
		model.CloseDB()
		setting.ResetAllToDefaults()
		middleware.ResetRateLimits()
	})
	password, _ := common.HashPassword("SMSFixturePassword92")
	admin := &model.User{Username: "sms-admin", Password: password, Role: common.RoleRoot, Status: 1, Group: "default", AffCode: common.GenerateAffCode(), AdminPasswordChangedAt: time.Now().Unix()}
	member := &model.User{Username: "sms-member", Password: password, Role: common.RoleUser, Status: 1, Group: "default", AffCode: common.GenerateAffCode()}
	for _, user := range []*model.User{admin, member} {
		if err := model.CreateUser(user); err != nil {
			t.Fatal(err)
		}
	}
	adminSession := middleware.Sessions.CreateConfirmed(admin, time.Hour, "127.0.0.1", "fixture")
	if !middleware.Sessions.SkipStaffTwoFA(adminSession, admin) {
		t.Fatal("could not prepare development administrator session")
	}
	memberSession := middleware.Sessions.CreateConfirmed(member, time.Hour, "127.0.0.1", "fixture")
	t.Cleanup(func() { middleware.Sessions.Remove(adminSession); middleware.Sessions.Remove(memberSession) })
	handler := router.New()
	secret := "SMSProviderFixtureSecret93"
	call := func(method, session string, body any, owner int64) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		path := "/api/system/sms-config"
		if method == "GET" {
			path += "?expected_user_id=" + strconv.FormatInt(owner, 10)
		}
		req := httptest.NewRequest(method, "http://example.test"+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://example.test")
		if session != "" {
			req.AddCookie(&http.Cookie{Name: "session", Value: session})
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if strings.Contains(recorder.Body.String(), secret) {
			t.Fatal("API exposed the provider secret")
		}
		return recorder
	}
	for _, tc := range []struct {
		session string
		status  int
	}{{"", 401}, {memberSession, 403}} {
		for _, method := range []string{"GET", "PUT"} {
			if w := call(method, tc.session, nil, admin.ID); w.Code != tc.status {
				t.Fatalf("authorization: %s %d", method, w.Code)
			}
		}
	}
	if w := call("GET", adminSession, nil, member.ID); w.Code != 409 {
		t.Fatal("cross-account read allowed")
	}
	if w := call("GET", adminSession, nil, admin.ID); w.Code != 200 || w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("initial configuration not available privately")
	}
	input := map[string]any{"expected_user_id": admin.ID, "expected_version": 0, "current_password": "wrong", "enabled": true, "access_key_id": "FixtureAccessKey123", "access_secret": secret, "sign_name": "SMS Test Sign", "template_code": "SMS_123456"}
	if w := call("PUT", adminSession, input, 0); w.Code != 403 {
		t.Fatal("password verification bypassed")
	}
	input["current_password"] = "SMSFixturePassword92"
	input["sign_name"] = ""
	if w := call("PUT", adminSession, input, 0); w.Code != 400 {
		t.Fatal("incomplete configuration enabled")
	}
	input["sign_name"] = "SMS Test Sign"
	middleware.ResetRateLimits()
	if w := call("PUT", adminSession, input, 0); w.Code != 200 {
		t.Fatalf("save failed: %d %s", w.Code, w.Body)
	}
	stored, err := model.GetSMSConfiguration()
	if err != nil || !strings.HasPrefix(stored.Key, "enc:v2:fixture:") || strings.Contains(stored.Key, secret) || stored.Version != 1 {
		t.Fatal("provider secret was not persisted as a versioned envelope")
	}
	if _, err := common.DecryptCredential(stored.Key, "upstream"); err == nil {
		t.Fatal("SMS secret accepted as a channel credential")
	}
	for _, h := range []http.HandlerFunc{controller.ListOptions, controller.OptionHistory} {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		h(w, req)
		if strings.Contains(w.Body.String(), secret) || strings.Contains(w.Body.String(), stored.Key) {
			t.Fatal("option APIs exposed SMS credentials")
		}
	}
	if w := call("GET", adminSession, nil, admin.ID); w.Code != 200 || strings.Contains(w.Body.String(), stored.Key) || !strings.Contains(w.Body.String(), `"secret_configured":true`) || !strings.Contains(w.Body.String(), `"effective_enabled":true`) {
		t.Fatal("configuration metadata did not reflect activation safely")
	}

	previousTransport := http.DefaultTransport
	if !service.SMSAvailableForCountry("CN") || service.SMSAvailableForCountry("US") {
		t.Fatal("direct SMS capabilities do not match supported countries")
	}
	t.Cleanup(func() { http.DefaultTransport = previousTransport })
	hits := 0
	responseCode := "OK"
	transportFailure := false
	http.DefaultTransport = smsTransport(func(req *http.Request) (*http.Response, error) {
		hits++
		if req.URL.Host != "dysmsapi.aliyuncs.com" || req.URL.Scheme != "https" || req.Method != "POST" {
			t.Fatal("unexpected provider destination")
		}
		if req.URL.Query().Get("PhoneNumbers") != "13812345678" || req.URL.Query().Get("TemplateCode") != "SMS_123456" || req.URL.Query().Get("TemplateParam") != `{"code":"123456"}` || strings.Contains(req.URL.RawQuery, "+") {
			t.Fatal("provider request contract failed")
		}
		fields := []string{"host", "x-acs-action", "x-acs-content-sha256", "x-acs-date", "x-acs-signature-nonce", "x-acs-version"}
		var canonical strings.Builder
		for _, field := range fields {
			value := req.Header.Get(field)
			if field == "host" {
				value = req.URL.Host
			}
			if value == "" {
				t.Fatal("missing signed provider header")
			}
			canonical.WriteString(field + ":" + strings.TrimSpace(value) + "\n")
		}
		var payload []byte
		if req.Body != nil {
			payload, _ = io.ReadAll(req.Body)
		}
		payloadHash := sha256.Sum256(payload)
		canonicalHash := sha256.Sum256([]byte(strings.Join([]string{req.Method, req.URL.EscapedPath(), req.URL.RawQuery, canonical.String(), strings.Join(fields, ";"), hex.EncodeToString(payloadHash[:])}, "\n")))
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte("ACS3-HMAC-SHA256\n" + hex.EncodeToString(canonicalHash[:])))
		expected := "ACS3-HMAC-SHA256 Credential=FixtureAccessKey123,SignedHeaders=" + strings.Join(fields, ";") + ",Signature=" + hex.EncodeToString(mac.Sum(nil))
		if req.Header.Get("Authorization") != expected {
			t.Fatal("provider signature could not be independently verified")
		}
		if transportFailure {
			return nil, errors.New("transport failure with private URL " + secret)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"Code":"` + responseCode + `","Message":"` + secret + `"}`))}, nil
	})
	if hits != 0 {
		t.Fatal("saving configuration sent an SMS")
	}
	if err := service.SendSMSVerification("+8613812345678", "123456"); err != nil {
		t.Fatal(err)
	}
	responseCode = "isv.BUSINESS_LIMIT_CONTROL"
	if err := service.SendSMSVerification("+8613812345678", "123456"); !errors.Is(err, service.ErrSMSRateLimited) {
		t.Fatal("provider throttling not classified")
	}
	transportFailure = true
	if err := service.SendSMSVerification("+8613812345678", "123456"); !errors.Is(err, service.ErrSMSSendFailed) || strings.Contains(err.Error(), secret) {
		t.Fatal("provider failure exposed private request data")
	}
	beforeHits := hits
	if service.SendSMSVerification("+12025550100", "123456") == nil || hits != beforeHits {
		t.Fatal("unsupported phone region reached this provider")
	}

	input["expected_version"], input["access_secret"], input["enabled"] = 1, "", false
	middleware.ResetRateLimits()
	if w := call("PUT", adminSession, input, 0); w.Code != 200 {
		t.Fatal("blank-secret preservation failed")
	}
	current, _ := model.GetSMSConfiguration()
	if current.Key != stored.Key || current.Version != 2 || service.IsSMSEnabled() {
		t.Fatal("secret preservation or live disable failed")
	}
	if w := call("PUT", adminSession, input, 0); w.Code != 409 {
		t.Fatal("stale configuration overwrote newer version")
	}
	t.Setenv("STARHUB_SMS_BRIDGE_URL", "http://127.0.0.1:1/send")
	t.Setenv("STARHUB_SMS_BRIDGE_TOKEN", "bridge-fixture")
	if err := service.ReloadSMSConfiguration(); err != nil {
		t.Fatal(err)
	}
	input["expected_version"] = 2
	if w := call("PUT", adminSession, input, 0); w.Code != 409 {
		t.Fatal("database UI overrode deployment-managed SMS")
	}
	if w := call("GET", adminSession, nil, admin.ID); !strings.Contains(w.Body.String(), `"source":"environment_bridge"`) {
		t.Fatal("deployment source not disclosed")
	}
	t.Setenv("STARHUB_SMS_BRIDGE_URL", "")
	t.Setenv("STARHUB_SMS_BRIDGE_TOKEN", "")

	t.Setenv("STARHUB_CREDENTIAL_KEYS", `{"fixture":"`+strings.Repeat("11", 32)+`","rotated":"`+strings.Repeat("22", 32)+`"}`)
	t.Setenv("STARHUB_CREDENTIAL_ACTIVE_KEY", "rotated")
	if count, err := model.MigrateCredentialStorage(true, "test", "Rotate SMS fixture"); err != nil || count != 1 {
		t.Fatalf("SMS credential rotation failed: %d %v", count, err)
	}
	t.Setenv("STARHUB_CREDENTIAL_KEYS", `{"rotated":"`+strings.Repeat("22", 32)+`"}`)
	if err := model.CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := model.InitDB(file); err != nil {
		t.Fatal(err)
	}
	if err := service.ReloadSMSConfiguration(); err != nil {
		t.Fatal(err)
	}
	restarted, _ := model.GetSMSConfiguration()
	if err := model.SaveSMSConfiguration(restarted, restarted.Version, stored.Key, admin); !errors.Is(err, model.ErrSMSConfigurationConflict) {
		t.Fatal("a save prepared before key rotation could overwrite rotated ciphertext")
	}
	plaintext, err := common.DecryptCredential(restarted.Key, "sms")
	if err != nil || plaintext != secret || restarted.Version != 2 || service.IsSMSEnabled() {
		t.Fatal("saved SMS configuration did not survive migration and restart")
	}
	input["clear_secret"] = true
	middleware.ResetRateLimits()
	if w := call("PUT", adminSession, input, 0); w.Code != 200 {
		t.Fatal("explicit credential clearing failed")
	}
	cleared, _ := model.GetSMSConfiguration()
	if cleared.Key != "" {
		t.Fatal("explicitly cleared secret remains in storage")
	}
}
