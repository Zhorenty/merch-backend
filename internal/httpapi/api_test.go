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

func TestPrivacyPage(t *testing.T) {
	e := setup(t)
	rec := e.do(t, http.MethodGet, "/privacy", "", nil)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, part := range []string{
		"Политика конфиденциальности",
		"Волошин Георгий Сергеевич",
		"zhorenty@gmail.com",
		"api.merch-wallet.ru",
		"Поддержка",
	} {
		if !strings.Contains(body, part) {
			t.Fatalf("missing %q in %s", part, body)
		}
	}
}

func TestSupportPage(t *testing.T) {
	e := setup(t)
	rec := e.do(t, http.MethodGet, "/support", "", nil)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, part := range []string{
		"Поддержка MERCH Касса",
		"Волошин Георгий Сергеевич",
		"zhorenty@gmail.com",
		"Что указать в письме",
	} {
		if !strings.Contains(body, part) {
			t.Fatalf("missing %q in %s", part, body)
		}
	}
}

func TestLoyaltyTermsPage(t *testing.T) {
	e := setup(t)
	rec := e.do(t, http.MethodGet, "/loyalty-terms", "", nil)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, part := range []string{"правила программы", "5%", "100", "50%", "Telegram @merch", "Срок действия баллов не ограничен"} {
		if !strings.Contains(body, part) {
			t.Fatalf("missing %q in %s", part, body)
		}
	}
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

func TestShiftReceiptsAndLogout(t *testing.T) {
	e := setup(t)
	leadTok := e.login(t, "/cashier/login", e.lead, "pass", 200)
	cashTok := e.login(t, "/cashier/login", e.cash, "pass", 200)
	adminCash := e.login(t, "/cashier/login", e.admin, "pass", 200)
	adminTok := e.login(t, "/admin/login", e.admin, "pass", 200)

	rec := e.do(t, http.MethodGet, "/cashier/receipts", cashTok, nil)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"receipts":[]}` {
		t.Fatalf("empty shift %d %s", rec.Code, rec.Body.String())
	}

	rec = e.do(t, http.MethodPost, "/public/enroll", "", map[string]string{"name": "Анна", "phone": "+79001112233"})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var enrolled struct {
		CustomerID string `json:"customer_id"`
		Barcode    string `json:"barcode"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &enrolled)
	rec = e.do(t, http.MethodPost, "/public/enroll", "", map[string]string{"name": "", "phone": "+79001112244"})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var unnamed struct {
		CustomerID string `json:"customer_id"`
		Barcode    string `json:"barcode"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &unnamed)

	rec = e.do(t, http.MethodPost, "/admin/adjust", adminTok, map[string]any{
		"barcode": enrolled.Barcode, "delta": 500, "reason": "seed",
	})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}

	rid := uuid.NewString()
	rec = e.do(t, http.MethodPost, "/cashier/commit", cashTok, map[string]any{
		"receipt_id": rid, "barcode": enrolled.Barcode, "receipt_amount_rub": 4500,
		"redeem_points": 500, "store_id": store.DefaultStoreID(),
	})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}

	start, _ := store.DayBounds(time.Now())
	otherID := uuid.NewString()
	if err := e.st.CreateStore(context.Background(), store.StoreRow{
		ID: otherID, Name: "Другая", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	staffID := e.staffID(t, e.cash)
	ctx := context.Background()
	err := e.st.InTx(ctx, func(tx *store.Tx) error {
		// SQLite stores time.Time as text. Keep the location UTC so the day filter matches commit rows.
		if err := tx.InsertReceipt(ctx, store.Receipt{
			ID: uuid.NewString(), StoreID: store.DefaultStoreID(), StaffID: staffID,
			CustomerID: unnamed.CustomerID, AmountRub: 100, Status: store.ReceiptCommitted,
			CreatedAt: start.Add(-time.Second).UTC(),
		}); err != nil {
			return err
		}
		return tx.InsertReceipt(ctx, store.Receipt{
			ID: uuid.NewString(), StoreID: otherID, StaffID: staffID,
			CustomerID: enrolled.CustomerID, AmountRub: 999, Status: store.ReceiptCommitted,
			CreatedAt: time.Now().UTC(),
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	rec = e.do(t, http.MethodGet, "/cashier/receipts", cashTok, nil)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var listed struct {
		Receipts []struct {
			ReceiptID    string `json:"receipt_id"`
			CreatedAt    string `json:"created_at"`
			Barcode      string `json:"barcode"`
			Name         string `json:"name"`
			AmountRub    int    `json:"amount_rub"`
			RedeemPoints int    `json:"redeem_points"`
			EarnPoints   int    `json:"earn_points"`
			Status       string `json:"status"`
			PointsAfter  int    `json:"points_after"`
		} `json:"receipts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Receipts) != 1 {
		t.Fatalf("shift list %+v", listed.Receipts)
	}
	got := listed.Receipts[0]
	if got.ReceiptID != rid || got.Barcode != enrolled.Barcode || got.Name != "Анна" ||
		got.AmountRub != 4500 || got.RedeemPoints != 500 || got.EarnPoints != 200 ||
		got.Status != "committed" || got.PointsAfter != 200 || got.CreatedAt == "" {
		t.Fatalf("receipt %+v", got)
	}
	if _, err := time.Parse(time.RFC3339, got.CreatedAt); err != nil {
		t.Fatal(err)
	}

	rec = e.do(t, http.MethodPost, "/cashier/refund", cashTok, map[string]string{"receipt_id": rid})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cashier refund %d %s", rec.Code, rec.Body.String())
	}
	var forbidden struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &forbidden)
	if forbidden.Error.Code != loyalty.CodeStaffForbidden {
		t.Fatalf("forbidden %+v", forbidden)
	}

	rec = e.do(t, http.MethodPost, "/cashier/refund", leadTok, map[string]string{"receipt_id": rid})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	rec = e.do(t, http.MethodGet, "/cashier/receipts", leadTok, nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Receipts) != 1 || listed.Receipts[0].Status != "refunded" || listed.Receipts[0].PointsAfter != 500 {
		t.Fatalf("after refund %+v", listed.Receipts)
	}

	rec = e.do(t, http.MethodPost, "/cashier/logout", cashTok, nil)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	rec = e.do(t, http.MethodPost, "/cashier/logout", cashTok, nil)
	if rec.Code != 200 {
		t.Fatalf("repeat logout %d %s", rec.Code, rec.Body.String())
	}
	rec = e.do(t, http.MethodGet, "/cashier/receipts", cashTok, nil)
	assertRevoked(t, rec)
	rec = e.do(t, http.MethodGet, "/admin/staff", adminTok, nil)
	if rec.Code != 200 {
		t.Fatalf("admin session should survive cashier logout: %d %s", rec.Code, rec.Body.String())
	}

	rec = e.do(t, http.MethodPost, "/cashier/logout", adminCash, nil)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	rec = e.do(t, http.MethodPost, "/admin/logout", adminTok, nil)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	rec = e.do(t, http.MethodPost, "/admin/logout", adminTok, nil)
	if rec.Code != 200 {
		t.Fatalf("repeat admin logout %d %s", rec.Code, rec.Body.String())
	}
	rec = e.do(t, http.MethodGet, "/admin/staff", adminTok, nil)
	assertRevoked(t, rec)
	rec = e.do(t, http.MethodGet, "/cashier/receipts", adminCash, nil)
	assertRevoked(t, rec)
}

func (e *env) staffID(t *testing.T, login string) string {
	t.Helper()
	st, err := e.st.GetStaffByLogin(context.Background(), login)
	if err != nil {
		t.Fatal(err)
	}
	return st.ID
}

func assertRevoked(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != loyalty.CodeUnauthorized || body.Error.Message != "Сессия отозвана" {
		t.Fatalf("revoked body %+v", body.Error)
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
