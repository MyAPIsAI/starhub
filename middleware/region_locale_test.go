package middleware

import (
	"net/http"
	"net/http/httptest"
	"starhub/common"
	"starhub/model"
	"testing"
	"time"
)

func TestAuthenticatedLanguageUsesCurrentAccountAndDeployment(t *testing.T) {
	setupAutoRoutingDB(t)
	t.Setenv("STARHUB_REGION_PROFILE", "locale")
	if err := model.SaveRegionProfile(&model.RegionProfile{ID: "locale", Name: "Locale", DefaultLanguage: "ar", SupportedLanguages: "ar,en", Enabled: true}, -1); err != nil {
		t.Fatal(err)
	}
	user := &model.User{Username: "locale-owner", Role: common.RoleUser, Status: common.StatusEnabled, Language: "ar", AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	session := Sessions.Create(user.ID, time.Hour)
	t.Cleanup(func() { Sessions.Remove(session) })
	for _, tc := range []struct{ account, header, want string }{{"ar", "en", "ar"}, {"", "en;q=0.9,ar;q=0.1", "en"}, {"ko", "fr", "ar"}} {
		if err := model.UpdateUserFields(user.ID, map[string]any{"language": tc.account}); err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("GET", "/api/user/self", nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: session})
		r.Header.Set("Accept-Language", tc.header)
		w := httptest.NewRecorder()
		UserAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := CtxLanguageValue(r.Context()); got != tc.want {
				t.Fatalf("resolved language %s, want %s", got, tc.want)
			}
			w.WriteHeader(204)
		})).ServeHTTP(w, r)
		if w.Code != 204 {
			t.Fatalf("locale request failed: %d", w.Code)
		}
	}
}
