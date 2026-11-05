package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"starhub/middleware"
	"starhub/model"
)

func TestStaffAndCustomerLoginRealmsAreIndependent(t *testing.T) {
	handler, root := staffFixture(t)
	root.enroll("root", "Root-temporary-123")
	if root.cookie.MaxAge != 8*60*60 {
		t.Fatal("verified staff cookie does not have an eight-hour lifetime")
	}
	staff, err := model.GetStaffByUsername("root")
	if err != nil {
		t.Fatal(err)
	}
	request := func(path string, data any, portal string, cookie *http.Cookie, want int) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(data)
		method := "POST"
		if data == nil {
			method = "GET"
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.RemoteAddr = root.ip
		r.Header.Set("Content-Type", "application/json")
		if portal != "" {
			r.Header.Set("X-StarHub-Portal", portal)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: got %d want %d: %s", path, w.Code, want, w.Body.String())
		}
		return w
	}
	credentials := map[string]string{"username": "root", "password": "Changed-Root-temporary-123"}
	rejected := request("/api/user/login", credentials, "workspace", nil, 401)
	if len(rejected.Result().Cookies()) != 0 {
		t.Fatal("customer login issued a session for staff credentials")
	}
	request("/api/user/register", map[string]string{"username": "root", "password": "Customer-password-123"}, "workspace", nil, 200)
	customer, err := model.GetUserByUsername("root")
	if err != nil || customer.ID == staff.ID || customer.AccountKind != "customer" {
		t.Fatal("same-name customer registration reused staff identity")
	}
	customerCredentials := map[string]string{"username": "root", "password": "Customer-password-123"}
	rejected = request("/api/admin/auth/login", customerCredentials, "staff", nil, 401)
	if len(rejected.Result().Cookies()) != 0 {
		t.Fatal("staff login issued a session for customer credentials")
	}
	request("/api/user/login", credentials, "workspace", nil, 401)
	accepted := request("/api/user/login", customerCredentials, "workspace", nil, 200)
	var customerCookie *http.Cookie
	for _, cookie := range accepted.Result().Cookies() {
		if cookie.Name == "session" {
			customerCookie = cookie
		}
	}
	if customerCookie == nil {
		t.Fatal("missing customer session")
	}
	request("/api/admin/auth/self", nil, "staff", customerCookie, 403)
	request("/api/user/self", nil, "", root.cookie, 401)
	request("/api/user/self", nil, "workspace", root.cookie, 200)
	request("/api/admin/auth/self", nil, "staff", root.cookie, 200)
	request("/api/user/self", nil, "workspace", customerCookie, 200)
	root.call("POST", "/api/admin/auth/logout", map[string]string{}, 200)
	state := root.call("POST", "/api/admin/auth/login", credentials, 200)
	if state["stage"] != "verify" || root.cookie.MaxAge != 5*60 {
		t.Fatal("re-login did not require a new OTP in a five-minute pending session")
	}
	root.call("GET", "/api/admin/auth/self", nil, 403)
	request("/api/user/self", nil, "workspace", root.cookie, 401)
	for _, verified := range []bool{false, true} {
		expired := middleware.Sessions.CreateStaffConfirmed(staff, verified, -time.Minute)
		if expired == "" {
			t.Fatal("missing expired fixture")
		}
		cookie := &http.Cookie{Name: "staff_session", Value: expired}
		request("/api/admin/auth/state", nil, "staff", cookie, 401)
		request("/api/admin/auth/self", nil, "staff", cookie, 401)
		request("/api/user/self", nil, "workspace", cookie, 401)
	}
}
