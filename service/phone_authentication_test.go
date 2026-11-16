package service_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"
	"starhub/common"
	"starhub/internal/router"
	"starhub/middleware"
	"starhub/model"
	"starhub/service"
	"starhub/setting"
)

type phoneFixtureProvider struct {
	codes map[string]string
	err   error
}

func (p *phoneFixtureProvider) Send(phone, code string) error {
	p.codes[phone] = code
	return p.err
}

func TestPhoneBindingAndLoginThroughProtectedRoutes(t *testing.T) {
	setting.ResetAllToDefaults()
	middleware.ResetRateLimits()
	if err := model.InitDB(filepath.Join(t.TempDir(), "phone.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		service.SetSMSProvider(nil)
		model.CloseDB()
		setting.ResetAllToDefaults()
		middleware.ResetRateLimits()
	})
	provider := &phoneFixtureProvider{codes: map[string]string{}}
	handler := router.New()
	password := "PhoneFixturePassword92"
	hash, _ := common.HashPassword(password)
	users := []*model.User{}
	sessions := []string{}
	for _, name := range []string{"phone-owner", "phone-other"} {
		user := &model.User{Username: name, Password: hash, Role: common.RoleUser, Status: common.StatusEnabled, Group: "default", AffCode: common.GenerateAffCode()}
		if err := model.CreateUser(user); err != nil {
			t.Fatal(err)
		}
		users = append(users, user)
		session := middleware.Sessions.CreateConfirmed(user, time.Hour, "192.0.2.8", "phone-fixture")
		sessions = append(sessions, session)
		t.Cleanup(func() { middleware.Sessions.Remove(session) })
	}
	call := func(method, path, session string, input any, want int) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(input)
		req := httptest.NewRequest(method, "http://example.test"+path, bytes.NewReader(raw))
		req.RemoteAddr = "192.0.2.8:12345"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://example.test")
		req.Header.Set("User-Agent", "phone-fixture")
		if session != "" {
			req.Header.Set("Cookie", "session="+session)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	resetBuckets := func() {
		t.Helper()
		if err := model.DB.Model(&model.SMSRequestBucket{}).Where("1 = 1").Updates(map[string]any{"started_at": time.Now().Add(-2 * time.Hour).Unix(), "count": 0}).Error; err != nil {
			t.Fatal(err)
		}
	}
	input := func(owner int, phone, code string) map[string]any {
		return map[string]any{"expected_user_id": users[owner].ID, "current_password": password, "phone": phone, "code": code}
	}
	service.SetSMSProvider(nil)
	w := call("GET", "/api/public/region", "", nil, 200)
	if !strings.Contains(w.Body.String(), `"phone_login_enabled":false`) {
		t.Fatal("disabled SMS advertised")
	}
	call("POST", "/api/user/phone-verification", "", map[string]any{"phone": "13812345678"}, 400)
	service.SetSMSProvider(provider)
	w = call("GET", "/api/public/region", "", nil, 200)
	if !strings.Contains(w.Body.String(), `"phone_login_enabled":true`) || strings.Contains(w.Body.String(), "access_secret") {
		t.Fatal("invalid public capabilities")
	}
	call("GET", "/api/user/phone?expected_user_id=1", "", nil, 401)
	call("POST", "/api/user/bind-phone", "", input(0, "13812345678", "123456"), 401)
	call("POST", "/api/user/phone-verification/bind", sessions[0], input(1, "13812345678", ""), 409)
	bad := input(0, "13812345678", "")
	bad["current_password"] = "incorrect"
	call("POST", "/api/user/phone-verification/bind", sessions[0], bad, 403)
	call("POST", "/api/user/phone-verification/bind", sessions[0], input(0, "invalid", ""), 400)
	if len(provider.codes) != 0 {
		t.Fatal("invalid request dispatched SMS")
	}
	call("POST", "/api/user/phone-verification/bind", sessions[0], input(0, "13812345678", ""), 200)
	code := provider.codes["+8613812345678"]
	if len(code) != 6 {
		t.Fatal("binding code missing")
	}
	call("POST", "/api/user/phone-verification/bind", sessions[0], input(0, "13812345678", ""), 429)
	call("POST", "/api/user/bind-phone", sessions[1], input(1, "13812345678", code), 400)
	call("POST", "/api/user/bind-phone", sessions[0], input(0, "13812345678", "wrong"), 400)
	call("POST", "/api/user/bind-phone", sessions[0], input(0, "13812345678", code), 200)
	w = call("GET", "/api/user/phone?expected_user_id="+strconv.FormatInt(users[0].ID, 10), sessions[0], nil, 200)
	if w.Header().Get("Cache-Control") != "private, no-store" || strings.Contains(w.Body.String(), "13812345678") || !strings.Contains(w.Body.String(), `"bound":true`) {
		t.Fatal("phone status not protected or masked")
	}
	call("POST", "/api/user/bind-phone", sessions[0], input(0, "13812345678", code), 400)
	events, err := model.ListUserSecurityEvents(users[0].ID)
	if err != nil || len(events) != 1 || events[0].Action != "phone_bound" {
		t.Fatal("binding event not recorded exactly once")
	}
	resetBuckets()
	known := call("POST", "/api/user/phone-verification", "", map[string]any{"phone": "13812345678"}, 200)
	unknown := call("POST", "/api/user/phone-verification", "", map[string]any{"phone": "13912345678"}, 409)
	if !strings.Contains(known.Body.String(), `"requested":true`) || !strings.Contains(unknown.Body.String(), `"message":"phone_not_linked"`) || strings.Contains(unknown.Body.String(), "retry_after") {
		t.Fatal("unlinked phone did not receive an explicit rejection without a resend timer")
	}
	if _, sent := provider.codes["+8613912345678"]; sent {
		t.Fatal("unlinked phone request reached the provider")
	}
	loginCode := provider.codes["+8613812345678"]
	wrong := "000000"
	if loginCode == wrong {
		wrong = "111111"
	}
	call("POST", "/api/user/login/phone", "", map[string]any{"phone": "13812345678", "code": wrong}, 401)
	w = call("POST", "/api/user/login/phone", "", map[string]any{"phone": "13812345678", "code": loginCode}, 200)
	var loginSession string
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "session" {
			loginSession = cookie.Value
			defer middleware.Sessions.Remove(loginSession)
		}
	}
	if loginSession == "" {
		t.Fatal("SMS login did not issue a session")
	}
	call("GET", "/api/user/self", loginSession, nil, 200)
	call("POST", "/api/user/login/phone", "", map[string]any{"phone": "13812345678", "code": loginCode}, 401)
	key, _ := common.GenerateTOTPSecret(users[0].Username)
	if err := model.DB.Create(&model.TwoFA{UserID: users[0].ID, Secret: key.Secret(), IsEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	resetBuckets()
	call("POST", "/api/user/phone-verification", "", map[string]any{"phone": "13812345678"}, 200)
	w = call("POST", "/api/user/login/phone", "", map[string]any{"phone": "13812345678", "code": provider.codes["+8613812345678"]}, 200)
	if !strings.Contains(w.Body.String(), `"2fa_required":true`) || !strings.Contains(w.Body.String(), users[0].Username) {
		t.Fatal("SMS bypassed second factor")
	}
	var challenge *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "session" {
			t.Fatal("session issued before second factor")
		}
		if cookie.HttpOnly && cookie.Value != "" {
			challenge = cookie
		}
	}
	if challenge == nil {
		t.Fatal("missing protected challenge")
	}
	totpCode, _ := totp.GenerateCode(key.Secret(), time.Now())
	call("POST", "/api/user/login/2fa", "", map[string]any{"username": users[0].Username, "code": totpCode}, 401)
	raw, _ := json.Marshal(map[string]any{"username": users[0].Username, "code": totpCode})
	req := httptest.NewRequest("POST", "http://example.test/api/user/login/2fa", bytes.NewReader(raw))
	req.RemoteAddr = "192.0.2.8:12345"
	req.Header.Set("Origin", "http://example.test")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "phone-fixture")
	req.AddCookie(challenge)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("second factor rejected: %d %s", w.Code, w.Body.String())
	}
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "session" {
			defer middleware.Sessions.Remove(cookie.Value)
		}
	}
	resetBuckets()
	provider.err = service.ErrSMSRateLimited
	call("POST", "/api/user/phone-verification", "", map[string]any{"phone": "13812345678"}, 429)
	resetBuckets()
	provider.err = errors.New("fixture sensitive provider failure")
	w = call("POST", "/api/user/phone-verification", "", map[string]any{"phone": "13812345678"}, 503)
	if strings.Contains(w.Body.String(), "sensitive") || strings.Contains(w.Body.String(), "13812345678") {
		t.Fatal("provider error leaked")
	}
	if err := model.DB.Create(&model.RegionProfile{ID: "default", Enabled: true, Timezone: "UTC", FeatureFlags: `{"sms":false}`}).Error; err != nil {
		t.Fatal(err)
	}
	w = call("GET", "/api/public/region", "", nil, 200)
	if !strings.Contains(w.Body.String(), `"phone_login_enabled":false`) {
		t.Fatal("region disabled SMS was advertised")
	}
	call("POST", "/api/user/phone-verification", "", map[string]any{"phone": "13812345678"}, 403)
	if err := model.DB.Model(&model.RegionProfile{}).Where("id = ?", "default").Updates(map[string]any{"feature_flags": "{}", "auth_plugins": "password"}).Error; err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/user/login/phone", "", map[string]any{"phone": "13812345678", "code": "123456"}, 403)
}

func TestPhoneBindingAtomicityAndPersistentLimits(t *testing.T) {
	file := filepath.Join(t.TempDir(), "phone-atomic.db")
	if err := model.InitDB(file); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { model.CloseDB() })
	users := []*model.User{}
	for _, name := range []string{"bind-a", "bind-b"} {
		u := &model.User{Username: name, Password: "fixture-hash", Role: common.RoleUser, Status: common.StatusEnabled, Group: "default", AffCode: common.GenerateAffCode()}
		if err := model.CreateUser(u); err != nil {
			t.Fatal(err)
		}
		users = append(users, u)
	}
	phone := "+8613812345678"
	for _, purpose := range []string{"owner-a", "owner-b", "expired", "stale", "register"} {
		expiry := time.Now().Add(time.Minute).Unix()
		if purpose == "expired" {
			expiry = time.Now().Add(-time.Second).Unix()
		}
		if err := model.CreatePhoneVerificationChallenge(phone, "123456", purpose, "fixture", expiry); err != nil {
			t.Fatal(err)
		}
	}
	if err := model.BindVerifiedUserPhone(users[0], phone, "123456", "expired"); !errors.Is(err, model.ErrVerificationInvalid) {
		t.Fatal("expired challenge accepted")
	}
	if err := model.BindVerifiedUserPhone(users[0], phone, "123456", "missing-owner"); !errors.Is(err, model.ErrVerificationInvalid) {
		t.Fatal("wrong purpose accepted")
	}
	stale := *users[0]
	stale.Password = "old-hash"
	if err := model.BindVerifiedUserPhone(&stale, phone, "123456", "stale"); !errors.Is(err, model.ErrPhoneBindingConflict) {
		t.Fatal("stale password proof accepted")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i, purpose := range []string{"owner-a", "owner-b"} {
		wg.Add(1)
		go func() { defer wg.Done(); results <- model.BindVerifiedUserPhone(users[i], phone, "123456", purpose) }()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	var count int64
	model.DB.Model(&model.UserPhoneIdentity{}).Where("phone_e164 = ?", phone).Count(&count)
	if successes != 1 || count != 1 {
		t.Fatal("concurrent binding violated uniqueness")
	}
	var remaining int64
	model.DB.Model(&model.PhoneVerificationChallenge{}).Where("purpose IN ? AND consumed_at IS NULL", []string{"owner-a", "owner-b"}).Count(&remaining)
	if remaining != 1 {
		t.Fatal("failed binding consumed the other challenge")
	}
	var bound model.UserPhoneIdentity
	if err := model.DB.Where("phone_e164 = ?", phone).First(&bound).Error; err != nil {
		t.Fatal(err)
	}
	var winner *model.User
	for _, user := range users {
		if user.ID == bound.UserID {
			winner = user
		}
	}
	newPhone := "+8613912345678"
	if err := model.CreatePhoneVerificationChallenge(newPhone, "123456", "new-phone", "fixture", time.Now().Add(time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	if err := model.BindVerifiedUserPhone(winner, newPhone, "123456", "new-phone"); !errors.Is(err, model.ErrPhoneBindingConflict) {
		t.Fatal("first binding endpoint replaced an existing phone")
	}
	stillBound, err := model.GetUserPhone(winner.ID)
	if err != nil || stillBound.PhoneE164 != phone {
		t.Fatal("failed first binding lost the original phone")
	}
	for range 5 {
		if err := model.BindVerifiedUserPhone(winner, newPhone, "000000", "new-phone"); !errors.Is(err, model.ErrVerificationInvalid) {
			t.Fatal(err)
		}
	}
	if err := model.BindVerifiedUserPhone(winner, newPhone, "123456", "new-phone"); !errors.Is(err, model.ErrVerificationLocked) {
		t.Fatal("failed attempts did not lock binding code")
	}
	limit := model.SMSRequestLimit{Scope: "fixture:phone", Window: time.Hour, Max: 1}
	if err := model.ReserveSMSRequest(limit); err != nil {
		t.Fatal(err)
	}
	if err := model.CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := model.InitDB(file); err != nil {
		t.Fatal(err)
	}
	if err := model.ReserveSMSRequest(limit); !errors.Is(err, model.ErrSMSRequestLimited) {
		t.Fatal("restart reset paid SMS limit")
	}
	// A rejected multi-bucket reservation must not consume unrelated budgets.
	fresh := model.SMSRequestLimit{Scope: "fixture:new", Window: time.Hour, Max: 1}
	if err := model.ReserveSMSRequest(fresh, limit); !errors.Is(err, model.ErrSMSRequestLimited) {
		t.Fatal(err)
	}
	if err := model.ReserveSMSRequest(fresh); err != nil {
		t.Fatal("failed reservation consumed a different budget")
	}
	concurrent := model.SMSRequestLimit{Scope: "fixture:concurrent", Window: time.Hour, Max: 1}
	results = make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- model.ReserveSMSRequest(concurrent) }()
	}
	wg.Wait()
	close(results)
	successes = 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, model.ErrSMSRequestLimited) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatal("concurrent callers bypassed SMS limit")
	}
	var event model.UserSecurityEvent
	if err := model.DB.First(&event).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("successful binding event missing")
	}
}
