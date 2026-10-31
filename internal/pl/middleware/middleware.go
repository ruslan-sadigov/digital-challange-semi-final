// Package middleware holds HTTP middleware: session loading, role checks, logging, panic recovery.
package middleware

import (
	"context"
	"log"
	"net/http"
	"slices"
	"time"

	"smartcity/internal/bl"
	"smartcity/internal/domain"
)

const CookieName = "sct_session"

type ctxKey struct{}

// WithSession stores a session in the context.
func WithSession(ctx context.Context, s bl.Session) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// SessionFrom returns the request's session, if any.
func SessionFrom(ctx context.Context) (bl.Session, bool) {
	s, ok := ctx.Value(ctxKey{}).(bl.Session)
	return s, ok
}

// TokenParser is the part of AuthService the middleware needs.
type TokenParser interface {
	ParseToken(token string) (bl.Session, error)
}

// LoadSession reads the session cookie (or "Authorization: Bearer") and, if valid, puts it in the context.
func LoadSession(p TokenParser) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ""
			if c, err := r.Cookie(CookieName); err == nil {
				token = c.Value
			} else if h := r.Header.Get("Authorization"); len(h) > 7 && h[:7] == "Bearer " {
				token = h[7:]
			}
			if token != "" {
				if s, err := p.ParseToken(token); err == nil {
					r = r.WithContext(WithSession(r.Context(), s))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole rejects requests without a session (401) or with a role outside roles (403).
// With no roles, any session is accepted.
func RequireRole(roles ...domain.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s, ok := SessionFrom(r.Context())
			if !ok {
				http.Error(w, `{"error":"login required"}`, http.StatusUnauthorized)
				return
			}
			if len(roles) > 0 && !slices.Contains(roles, s.Role) {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// DeviceKey protects bus GPS ingestion with a shared API key (X-Device-Key header).
func DeviceKey(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if key == "" || r.Header.Get("X-Device-Key") != key {
				http.Error(w, `{"error":"invalid device key"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController (and the WebSocket upgrader via Hijack) reach the real writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Logger logs method, path, status and duration. WebSocket upgrades bypass the recorder.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") == "websocket" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		if r.URL.Path != "/healthz" {
			log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
		}
	})
}

// Recover turns panics into 500s.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Printf("panic: %v", v)
				http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Chain applies middleware so the first one listed runs first.
func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
