package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/setting"
)

// newTestDB boots an isolated SQLite database for one test and restores the
// process-wide state the handlers read: rate-limit buckets and every setting
// the option handlers can mutate. Without this a test that flips a ratio
// would poison whichever test runs next in the same binary.
func newTestDB(t *testing.T) {
	t.Helper()
	// Reset before seeding: a previous test may have failed before cleanup
	// and left dirty in-memory state that SeedOptions would persist.
	setting.ResetAllToDefaults()
	middleware.ResetRateLimits()
	middleware.ResetTokenRateLimits()
	middleware.ResetModelRateLimits()

	if err := model.InitDB(filepath.Join(t.TempDir(), "controller.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = model.CloseDB()
		setting.ResetAllToDefaults()
	})
	if err := model.LoadOptionsToSettings(); err != nil {
		t.Fatalf("LoadOptionsToSettings failed: %v", err)
	}
	if err := model.Seed("123456"); err != nil {
		t.Fatalf("Seed failed: %v", err)
	}
}

// newUser creates a user row and returns it. The password is always "secret"
// so login tests can reuse it.
func newUser(t *testing.T, username string, role int, quota int64) *model.User {
	t.Helper()
	hashed, err := common.HashPassword("secret")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	u := &model.User{
		Username: username,
		Password: hashed,
		Role:     role,
		Status:   common.StatusEnabled,
		Quota:    quota,
		Group:    common.DefaultGroup,
		AffCode:  common.GenerateAffCode(),
	}
	if err := model.CreateUser(u); err != nil {
		t.Fatalf("CreateUser(%s) failed: %v", username, err)
	}
	return u
}

// mustToken inserts a token owned by userID with the given balance.
func mustToken(t *testing.T, userID, remainQuota int64) *model.Token {
	t.Helper()
	tk := &model.Token{
		UserID:      userID,
		Name:        "key",
		Status:      common.StatusEnabled,
		ExpiredTime: -1,
		RemainQuota: remainQuota,
	}
	if err := model.InsertToken(tk); err != nil {
		t.Fatalf("InsertToken failed: %v", err)
	}
	return tk
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

// testRecorder records what a helper wrote when it is exercised directly,
// without going through a full handler.
type testRecorder struct {
	header http.Header
	body   []byte
	status int
}

func (r *testRecorder) Header() http.Header {
	if r.header == nil {
		r.header = http.Header{}
	}
	return r.header
}

func (r *testRecorder) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return len(b), nil
}

func (r *testRecorder) WriteHeader(status int) { r.status = status }

// sessionTTLForTest is long enough that a slow CI run cannot expire a session
// mid-test.
const sessionTTLForTest = time.Hour

func repeat(s string, n int) string { return strings.Repeat(s, n) }

type testRequest struct {
	method  string
	target  string
	body    string
	userID  int64
	role    int
	path    map[string]string
	headers map[string]string
}

// call drives a handler the way the router does but without opening a
// listening socket: the request carries the identity context the auth
// middleware would normally inject, and path wildcards are set explicitly
// because there is no mux to populate them.
func call(h http.HandlerFunc, tr testRequest) *httptest.ResponseRecorder {
	if tr.method == "" {
		tr.method = http.MethodGet
	}
	r := httptest.NewRequest(tr.method, tr.target, strings.NewReader(tr.body))
	if tr.body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	ctx := r.Context()
	if tr.userID != 0 || tr.role != 0 {
		ctx = common.WithValues(ctx, common.CtxUserID, tr.userID, common.CtxRole, tr.role)
	}
	r = r.WithContext(ctx)
	for k, v := range tr.path {
		r.SetPathValue(k, v)
	}
	for k, v := range tr.headers {
		r.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h(rr, r)
	return rr
}

// apiEnvelope mirrors common.APIResponse for assertions. Data is kept raw so
// each test can decode it into the shape it expects.
type apiEnvelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func decodeEnvelope(t *testing.T, rr *httptest.ResponseRecorder) apiEnvelope {
	t.Helper()
	var env apiEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not a JSON API envelope: %v (body %q)", err, rr.Body.String())
	}
	return env
}

// expectStatus asserts the HTTP status and returns the decoded envelope.
func expectStatus(t *testing.T, rr *httptest.ResponseRecorder, want int) apiEnvelope {
	t.Helper()
	if rr.Code != want {
		t.Fatalf("status = %d, want %d (body %s)", rr.Code, want, rr.Body.String())
	}
	return decodeEnvelope(t, rr)
}

// expectError asserts a failed envelope whose message contains want, which is
// how every handler reports a validation problem.
func expectError(t *testing.T, rr *httptest.ResponseRecorder, status int, wantSubstring string) {
	t.Helper()
	env := expectStatus(t, rr, status)
	if env.Success {
		t.Fatalf("expected failure envelope, got success (body %s)", rr.Body.String())
	}
	if !strings.Contains(env.Message, wantSubstring) {
		t.Fatalf("message %q does not contain %q", env.Message, wantSubstring)
	}
}

// decodeData unmarshals the envelope's data payload into v.
func decodeData(t *testing.T, rr *httptest.ResponseRecorder, v any) {
	t.Helper()
	env := expectStatus(t, rr, http.StatusOK)
	if !env.Success {
		t.Fatalf("expected success envelope, got %q", env.Message)
	}
	if err := json.Unmarshal(env.Data, v); err != nil {
		t.Fatalf("decoding data %s: %v", env.Data, err)
	}
}

// decodeJSON unmarshals a raw response body, for endpoints that return their
// own envelope (the relay surface uses provider-shaped bodies, not the
// management API wrapper).
func decodeJSON(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("decoding body %s: %v", body, err)
	}
}
