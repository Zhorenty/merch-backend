package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"merch/backend/internal/loyalty"
	"merch/backend/internal/store"
)

type enrollReq struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

func (s *Server) postPublicEnroll(w http.ResponseWriter, r *http.Request) {
	var req enrollReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	s.enroll(w, r, req, true)
}

func (s *Server) postCashierEnroll(w http.ResponseWriter, r *http.Request) {
	var req enrollReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	s.enroll(w, r, req, false)
}

func (s *Server) enroll(w http.ResponseWriter, r *http.Request, req enrollReq, setCookie bool) {
	cookieID := ""
	if c, err := r.Cookie("merch_cid"); err == nil {
		cookieID = c.Value
	}
	res, err := s.Loyalty.Enroll(r.Context(), loyalty.EnrollInput{
		Name:     req.Name,
		Phone:    req.Phone,
		CookieID: cookieID,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	if setCookie {
		http.SetCookie(w, &http.Cookie{
			Name:     "merch_cid",
			Value:    res.Customer.ID,
			Path:     "/",
			MaxAge:   10 * 365 * 24 * 3600,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}
	googleURL := ""
	if s.Google != nil {
		u, err := s.Google.SaveURL(res.Customer)
		if err == nil {
			googleURL = u
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"customer_id":     res.Customer.ID,
		"barcode":         res.Customer.Barcode,
		"apple_url":       s.Cfg.APIBaseURL + "/public/passes/apple/" + res.Customer.ID + ".pkpass",
		"google_save_url": googleURL,
		"add_page":        s.Cfg.PublicBaseURL + "/card/add/" + res.Customer.ID,
		"created":         res.Created,
	})
}

func (s *Server) getApplePass(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(chi.URLParam(r, "id"), ".pkpass")
	cust, err := s.loadCustomer(r, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.writePKPass(w, r, cust)
}

func (s *Server) loadCustomer(r *http.Request, id string) (store.Customer, error) {
	c, err := s.Store.GetCustomerByID(r.Context(), id)
	if err == nil {
		return c, nil
	}
	if !store.IsNoRows(err) {
		return store.Customer{}, err
	}
	c, err = s.Store.GetCustomerByBarcode(r.Context(), id)
	if store.IsNoRows(err) {
		return store.Customer{}, loyalty.Err(loyalty.CodeCustomerNotFound, "Клиент не найден")
	}
	return c, err
}

func (s *Server) writePKPass(w http.ResponseWriter, r *http.Request, c store.Customer) {
	if s.Apple == nil {
		writeError(w, http.StatusServiceUnavailable, loyalty.CodeInternal, "Apple Wallet не настроен")
		return
	}
	ims := r.Header.Get("If-Modified-Since")
	if ims != "" {
		if t, err := time.Parse(time.RFC1123, ims); err == nil && !c.UpdatedAt.After(t.Add(time.Second)) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	body, err := s.Apple.BuildPKPass(r.Context(), c, "Баллы")
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.pkpass")
	w.Header().Set("Content-Disposition", `attachment; filename="merch.pkpass"`)
	w.Header().Set("Last-Modified", c.UpdatedAt.UTC().Format(time.RFC1123))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
