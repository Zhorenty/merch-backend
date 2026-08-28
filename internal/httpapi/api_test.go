package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"merch/backend/internal/auth"
	"merch/backend/internal/config"
	"merch/backend/internal/httpapi"
	"merch/backend/internal/jobs"
	"merch/backend/internal/loyalty"
	"merch/backend/internal/store"
	"merch/backend/internal/wallet"
	"merch/backend/internal/wallet/apple"
	googlew "merch/backend/internal/wallet/google"
)

type env struct {
	h     http.Handler
	st    *store.Store
	admin string
	cash  string
	lead  string
	inact string
	cfg   config.Config
}

func setup(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite:file:api_"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cfg := config.Config{
		APIBaseURL:       "http://example",
		PublicBaseURL:    "http://example",
		CashierJWTSecret: "cashier-secret",
		AdminJWTSecret:   "admin-secret",
		CashierJWTTTL:    12 * time.Hour,
		AdminJWTTTL:      12 * time.Hour,
		ApplePassTypeID:  "pass.com.merch.loyalty",
		TermsURL:         "http://example/terms",
		SupportContact:   "Telegram @merch",
		AppMinSupported:  "1.0.0",
		AppDownloadURL:   "http://example/app.apk",
		PassBGColor:      "rgb(26,26,26)",
		PassFGColor:      "rgb(245,245,245)",
		PassLabelColor:   "rgb(180,180,180)",
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ap := apple.New(cfg, log)
	goog := googlew.New(cfg, log)
	wlt := &wallet.Composite{Log: log, Store: st, Apple: ap, Google: goog}
	q := jobs.New(st, wlt, log)
	loy := &loyalty.Service{Store: st, Jobs: q}
	srv, err := httpapi.New(cfg, log, st, loy, wlt, q, ap, goog)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{h: srv.Handler(), st: st, cfg: cfg}
	e.admin = mustStaff(t, st, "admin", auth.RoleAdmin, true, "pass")
	e.cash = mustStaff(t, st, "cash", auth.RoleCashier, true, "pass")
	e.lead = mustStaff(t, st, "lead", auth.RoleShiftLead, true, "pass")
	e.inact = mustStaff(t, st, "gone", auth.RoleCashier, false, "pass")
	return e
}

func mustStaff(t *testing.T, st *store.Store, login, role string, active bool, pass string) string {
	t.Helper()
	h, _ := auth.HashSecret(pass)
	s := store.Staff{
		ID: uuid.NewString(), StoreID: store.DefaultStoreID(), Login: login, Name: login,
		PasswordHash: h, Role: role, Active: active, CreatedAt: time.Now().UTC(),
	}
	if err := st.CreateStaff(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	return login
}

func (e *env) login(t *testing.T, path, login, pass string, code int) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"login": login, "password": pass})
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != code {
		t.Fatalf("%s login status=%d body=%s", login, rec.Code, rec.Body.String())
	}
	if code != 200 {
		return ""
	}
	var out struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out.Token
}

func (e *env) do(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func TestInactiveStaffCannotLogin(t *testing.T) {
	e := setup(t)
	e.login(t, "/cashier/login", e.inact, "pass", http.StatusUnauthorized)
}

func TestCommitQuoteRefundHTTP(t *testing.T) {
	e := setup(t)
	tok := e.login(t, "/cashier/login", e.lead, "pass", 200)
	admin := e.login(t, "/admin/login", e.admin, "pass", 200)

	rec := e.do(t, http.MethodPost, "/public/enroll", "", map[string]string{"name": "Анна", "phone": "+79001112233"})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var enrolled struct {
		CustomerID string `json:"customer_id"`
		Barcode    string `json:"barcode"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &enrolled)

	rec = e.do(t, http.MethodPost, "/admin/adjust", admin, map[string]any{
		"barcode": enrolled.Barcode, "delta": 500, "reason": "seed",
	})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}

	rec = e.do(t, http.MethodPost, "/cashier/quote-redeem", tok, map[string]any{
		"barcode": enrolled.Barcode, "receipt_amount_rub": 4500, "requested_points": 500,
	})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var q loyalty.Quote
	_ = json.Unmarshal(rec.Body.Bytes(), &q)
	if !q.Allowed || q.EarnPoints != 200 {
		t.Fatalf("quote %+v", q)
	}

	rid := uuid.NewString()
	rec = e.do(t, http.MethodPost, "/cashier/commit", tok, map[string]any{
		"receipt_id": rid, "barcode": enrolled.Barcode, "receipt_amount_rub": 4500,
		"redeem_points": 500, "store_id": store.DefaultStoreID(),
	})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var commit loyalty.CommitResult
	_ = json.Unmarshal(rec.Body.Bytes(), &commit)
	if commit.EarnPoints != 200 || commit.Points != 200 {
		t.Fatalf("commit %+v", commit)
	}

	rec = e.do(t, http.MethodPost, "/cashier/commit", tok, map[string]any{
		"receipt_id": rid, "barcode": enrolled.Barcode, "receipt_amount_rub": 4500,
		"redeem_points": 500, "store_id": store.DefaultStoreID(),
	})
	_ = json.Unmarshal(rec.Body.Bytes(), &commit)
	if rec.Code != 200 || !commit.IdempotentReplay {
		t.Fatalf("replay %d %s", rec.Code, rec.Body.String())
	}

	rec = e.do(t, http.MethodPost, "/cashier/quote-redeem", tok, map[string]any{
		"barcode": enrolled.Barcode, "receipt_amount_rub": 4500, "requested_points": 50,
	})
	_ = json.Unmarshal(rec.Body.Bytes(), &q)
	if q.Allowed || q.Code != loyalty.CodeBelowMinRedeem {
		t.Fatalf("min quote %+v", q)
	}

	cashTok := e.login(t, "/cashier/login", e.cash, "pass", 200)
	rec = e.do(t, http.MethodPost, "/cashier/refund", cashTok, map[string]string{"receipt_id": rid})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cashier refund %d", rec.Code)
	}
	rec = e.do(t, http.MethodPost, "/cashier/refund", tok, map[string]string{"receipt_id": rid})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}

	rec = e.do(t, http.MethodGet, "/card/add/"+enrolled.CustomerID, "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), enrolled.Barcode) {
		t.Fatalf("add page %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/card/add/"+enrolled.CustomerID, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 12; HUAWEI)")
	rec2 := httptest.NewRecorder()
	e.h.ServeHTTP(rec2, req)
	if rec2.Code != 200 || !strings.Contains(rec2.Body.String(), "Huawei") {
		t.Fatalf("huawei page: %s", rec2.Body.String())
	}

	rec = e.do(t, http.MethodGet, "/public/passes/apple/"+enrolled.CustomerID+".pkpass", "", nil)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/vnd.apple.pkpass" {
		t.Fatalf("pkpass %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}

	rec = e.do(t, http.MethodGet, "/cashier/app-version", "", nil)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
}

func TestRepeatEnrollHTTPCookieAndPhone(t *testing.T) {
	e := setup(t)
	req := httptest.NewRequest(http.MethodPost, "/public/enroll", strings.NewReader(`{"name":"A","phone":"+79005551122"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var a struct {
		ID      string `json:"customer_id"`
		Created bool   `json:"created"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &a)
	req2 := httptest.NewRequest(http.MethodPost, "/public/enroll", strings.NewReader(`{"name":"B","phone":"+79005551122"}`))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	e.h.ServeHTTP(rec2, req2)
	var b struct {
		ID      string `json:"customer_id"`
		Created bool   `json:"created"`
	}
	_ = json.Unmarshal(rec2.Body.Bytes(), &b)
	if b.Created || a.ID != b.ID {
		t.Fatalf("duplicate phone created second card: %+v %+v", a, b)
	}
}
