package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"smartcity/internal/bl"
	"smartcity/internal/domain"
)

type fakeParser map[string]bl.Session

func (f fakeParser) ParseToken(tok string) (bl.Session, error) {
	if s, ok := f[tok]; ok {
		return s, nil
	}
	return bl.Session{}, domain.ErrUnauthorized
}

var ok = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })

func TestRequireRole(t *testing.T) {
	parser := fakeParser{
		"op":    {UserID: 1, Role: domain.RoleOperator},
		"guest": {UserID: 2, Role: domain.RoleGuest},
	}
	h := Chain(ok, LoadSession(parser), RequireRole(domain.RoleOperator))
	cases := []struct {
		name   string
		cookie string
		bearer string
		want   int
	}{
		{"no session", "", "", 401},
		{"invalid token", "nope", "", 401},
		{"guest forbidden", "guest", "", 403},
		{"operator cookie", "op", "", 418},
		{"operator bearer", "", "op", 418},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if c.cookie != "" {
				req.AddCookie(&http.Cookie{Name: CookieName, Value: c.cookie})
			}
			if c.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+c.bearer)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Fatalf("status %d, want %d", rec.Code, c.want)
			}
		})
	}
	// RequireRole() with no roles accepts any session
	any := Chain(ok, LoadSession(parser), RequireRole())
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "guest"})
	rec := httptest.NewRecorder()
	any.ServeHTTP(rec, req)
	if rec.Code != 418 {
		t.Fatalf("any session: %d", rec.Code)
	}
}

func TestDeviceKey(t *testing.T) {
	h := DeviceKey("k")(ok)
	for _, c := range []struct {
		key  string
		want int
	}{{"", 401}, {"bad", 401}, {"k", 418}} {
		req := httptest.NewRequest("POST", "/", nil)
		req.Header.Set("X-Device-Key", c.key)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Fatalf("key %q: %d want %d", c.key, rec.Code, c.want)
		}
	}
	// empty configured key never authorises
	rec := httptest.NewRecorder()
	DeviceKey("")(ok).ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))
	if rec.Code != 401 {
		t.Fatal("empty key must reject")
	}
}

func TestRecover(t *testing.T) {
	h := Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 500 {
		t.Fatalf("got %d", rec.Code)
	}
}
