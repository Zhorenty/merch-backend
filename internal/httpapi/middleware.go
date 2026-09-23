package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/time/rate"
	"merch/backend/internal/auth"
	"merch/backend/internal/loyalty"
	"merch/backend/internal/store"
)

type ctxKey int

const (
	ctxStaff ctxKey = iota
	ctxClaims
)

func withTimeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func requestLog(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			reqID := ww.Header().Get("X-Request-Id")
			if reqID == "" {
				reqID = randomID()
				ww.Header().Set("X-Request-Id", reqID)
			}
			next.ServeHTTP(ww, r)
			log.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"ms", time.Since(start).Milliseconds(),
				"request_id", reqID,
			)
		})
	}
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func cors(origins []string) func(http.Handler) http.Handler {
	allow := map[string]struct{}{}
	for _, o := range origins {
		allow[o] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if _, ok := allow[origin]; ok || origin == "null" {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func maxBody(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}

type ipLimiter struct {
	mu    sync.Mutex
	m     map[string]*rate.Limiter
	r     rate.Limit
	burst int
}

func newIPLimiter(perMin float64, burst int) *ipLimiter {
	return &ipLimiter{m: map[string]*rate.Limiter{}, r: rate.Limit(perMin / 60), burst: burst}
}

func (l *ipLimiter) allow(ip string) bool {
	l.mu.Lock()
	lim, ok := l.m[ip]
	if !ok {
		lim = rate.NewLimiter(l.r, l.burst)
		l.m[ip] = lim
	}
	l.mu.Unlock()
	return lim.Allow()
}

func (s *Server) rateLimit(l *ipLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			if ip == "" {
				ip = r.RemoteAddr
			}
			if !l.allow(ip) {
				writeError(w, http.StatusTooManyRequests, loyalty.CodeInvalidRequest, "Слишком много запросов, попробуйте позже")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) requireStaff(kind string, secret string) func(http.Handler) http.Handler {
	return s.authenticateStaff(kind, secret, false)
}

// requireStaffAllowRevoked accepts a still-signed JWT after RevokeSession.
// Logout must answer 200 on a repeat call with the same Bearer.
func (s *Server) requireStaffAllowRevoked(kind string, secret string) func(http.Handler) http.Handler {
	return s.authenticateStaff(kind, secret, true)
}

func (s *Server) authenticateStaff(kind, secret string, allowRevoked bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := bearer(r.Header.Get("Authorization"))
			if raw == "" {
				writeError(w, http.StatusUnauthorized, loyalty.CodeUnauthorized, "Нужна авторизация")
				return
			}
			claims, err := auth.Parse(secret, raw)
			if err != nil || claims.Kind != kind {
				writeError(w, http.StatusUnauthorized, loyalty.CodeUnauthorized, "Недействительный токен")
				return
			}
			ok, err := s.Store.SessionValid(r.Context(), claims.ID)
			if err != nil {
				writeErr(w, err)
				return
			}
			if !ok && !allowRevoked {
				writeError(w, http.StatusUnauthorized, loyalty.CodeUnauthorized, "Сессия отозвана")
				return
			}
			st, err := s.Store.GetStaffByID(r.Context(), claims.Subject)
			if err != nil || !st.Active {
				writeError(w, http.StatusUnauthorized, loyalty.CodeUnauthorized, "Сотрудник неактивен")
				return
			}
			ctx := context.WithValue(r.Context(), ctxStaff, st)
			ctx = context.WithValue(ctx, ctxClaims, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func staffFrom(r *http.Request) store.Staff {
	st, _ := r.Context().Value(ctxStaff).(store.Staff)
	return st
}

func claimsFrom(r *http.Request) *auth.Claims {
	c, _ := r.Context().Value(ctxClaims).(*auth.Claims)
	return c
}

func bearer(h string) string {
	const p = "Bearer "
	if strings.HasPrefix(h, p) {
		return strings.TrimSpace(h[len(p):])
	}
	if strings.HasPrefix(h, "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func applePassToken(h string) string {
	const p = "ApplePass "
	if strings.HasPrefix(h, p) {
		return strings.TrimSpace(h[len(p):])
	}
	return ""
}
