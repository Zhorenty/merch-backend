package httpapi

import (
	"net/http"

	"merch/backend/internal/auth"
	"merch/backend/internal/loyalty"
	"merch/backend/internal/store"
)

type loginReq struct {
	Login    string `json:"login"`
	Password string `json:"password"`
	PIN      string `json:"pin"`
}

type barcodeReq struct {
	Barcode string `json:"barcode"`
}

type quoteReq struct {
	Barcode          string `json:"barcode"`
	ReceiptAmountRub int    `json:"receipt_amount_rub"`
	RequestedPoints  int    `json:"requested_points"`
}

type commitReq struct {
	ReceiptID        string `json:"receipt_id"`
	Barcode          string `json:"barcode"`
	ReceiptAmountRub int    `json:"receipt_amount_rub"`
	RedeemPoints     int    `json:"redeem_points"`
	StoreID          string `json:"store_id"`
}

type refundReq struct {
	ReceiptID string `json:"receipt_id"`
}

func (s *Server) postCashierLogin(w http.ResponseWriter, r *http.Request) {
	s.login(w, r, auth.KindCashier, s.Cfg.CashierJWTSecret, s.Cfg.CashierJWTTTL, false)
}

func (s *Server) postAdminLogin(w http.ResponseWriter, r *http.Request) {
	s.login(w, r, auth.KindAdmin, s.Cfg.AdminJWTSecret, s.Cfg.AdminJWTTTL, true)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request, kind, secret string, ttl interface{ String() string }, adminOnly bool) {
	_ = ttl
	var req loginReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	st, err := s.Store.GetStaffByLogin(r.Context(), req.Login)
	if store.IsNoRows(err) {
		writeError(w, http.StatusUnauthorized, loyalty.CodeUnauthorized, "Неверный логин или пароль")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if !st.Active {
		writeError(w, http.StatusUnauthorized, loyalty.CodeUnauthorized, "Сотрудник неактивен")
		return
	}
	if adminOnly && st.Role != auth.RoleAdmin {
		writeError(w, http.StatusForbidden, loyalty.CodeStaffForbidden, "Недостаточно прав")
		return
	}
	ok := auth.CheckSecret(st.PasswordHash, req.Password)
	if !ok && req.PIN != "" {
		ok = auth.CheckSecret(st.PINHash, req.PIN)
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, loyalty.CodeUnauthorized, "Неверный логин или пароль")
		return
	}
	dur := s.Cfg.CashierJWTTTL
	if kind == auth.KindAdmin {
		dur = s.Cfg.AdminJWTTTL
	}
	jti, err := auth.NewJTI()
	if err != nil {
		writeErr(w, err)
		return
	}
	token, exp, err := auth.Sign(secret, kind, st.ID, st.Login, st.Role, st.StoreID, jti, dur)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.Store.CreateSession(r.Context(), jti, st.ID, kind, exp); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_at": exp.UTC().Format(timeRFC3339()),
		"staff": map[string]any{
			"id":       st.ID,
			"name":     st.Name,
			"role":     st.Role,
			"store_id": st.StoreID,
		},
	})
}

func timeRFC3339() string { return "2006-01-02T15:04:05Z07:00" }

func (s *Server) getAppVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"min_supported": s.Cfg.AppMinSupported,
		"download_url":  s.Cfg.AppDownloadURL,
	})
}

func (s *Server) postLookup(w http.ResponseWriter, r *http.Request) {
	var req barcodeReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	res, err := s.Loyalty.Lookup(r.Context(), req.Barcode)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"customer_id": res.Customer.ID,
		"name":        res.Customer.DisplayName,
		"points":      res.Customer.Points,
		"redeem_min":  res.Settings.RedeemMin,
		"redeem_rate": res.Settings.RedeemRate,
		"can_redeem":  res.CanRedeem,
	})
}

func (s *Server) postQuote(w http.ResponseWriter, r *http.Request) {
	var req quoteReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	q, err := s.Loyalty.Quote(r.Context(), req.Barcode, req.ReceiptAmountRub, req.RequestedPoints)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, q)
}

func (s *Server) postCommit(w http.ResponseWriter, r *http.Request) {
	st := staffFrom(r)
	var req commitReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	storeID := req.StoreID
	if storeID == "" {
		storeID = st.StoreID
	}
	if storeID != st.StoreID && st.Role != auth.RoleAdmin {
		writeError(w, http.StatusForbidden, loyalty.CodeStaffForbidden, "Нельзя провести чек в чужой точке")
		return
	}
	if _, err := s.Store.GetStore(r.Context(), storeID); store.IsNoRows(err) {
		writeError(w, http.StatusUnprocessableEntity, loyalty.CodeInvalidRequest, "Точка не найдена")
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	res, err := s.Loyalty.Commit(r.Context(), loyalty.CommitInput{
		ReceiptID:    req.ReceiptID,
		Barcode:      req.Barcode,
		AmountRub:    req.ReceiptAmountRub,
		RedeemPoints: req.RedeemPoints,
		StoreID:      storeID,
		StaffID:      st.ID,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) postRefund(w http.ResponseWriter, r *http.Request) {
	st := staffFrom(r)
	if !auth.CanRefund(st.Role) {
		writeError(w, http.StatusForbidden, loyalty.CodeStaffForbidden, "Возврат доступен старшему смены или админу")
		return
	}
	var req refundReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	res, err := s.Loyalty.Refund(r.Context(), req.ReceiptID, st.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
