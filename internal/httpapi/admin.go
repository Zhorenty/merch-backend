package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"merch/backend/internal/auth"
	"merch/backend/internal/loyalty"
	"merch/backend/internal/store"
)

type adjustReq struct {
	CustomerID string `json:"customer_id"`
	Barcode    string `json:"barcode"`
	Delta      int    `json:"delta"`
	Reason     string `json:"reason"`
}

type staffReq struct {
	Login    string `json:"login"`
	Name     string `json:"name"`
	Password string `json:"password"`
	PIN      string `json:"pin"`
	Role     string `json:"role"`
	StoreID  string `json:"store_id"`
	Active   *bool  `json:"active"`
}

type storeReq struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

func (s *Server) postAdjust(w http.ResponseWriter, r *http.Request) {
	st := staffFrom(r)
	if !auth.IsAdmin(st.Role) {
		writeError(w, http.StatusForbidden, loyalty.CodeStaffForbidden, "Недостаточно прав")
		return
	}
	var req adjustReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	ref := req.CustomerID
	if ref == "" {
		ref = req.Barcode
	}
	c, err := s.Loyalty.Adjust(r.Context(), ref, req.Delta, req.Reason, st.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"customer_id": c.ID,
		"barcode":     c.Barcode,
		"points":      c.Points,
	})
}

func (s *Server) staffJSON(ctx context.Context, st store.Staff) map[string]any {
	name := st.StoreName
	if name == "" && st.StoreID != "" {
		if row, err := s.Store.GetStore(ctx, st.StoreID); err == nil {
			name = row.Name
		}
	}
	return map[string]any{
		"id":         st.ID,
		"store_id":   st.StoreID,
		"store_name": name,
		"login":      st.Login,
		"name":       st.Name,
		"role":       st.Role,
		"active":     st.Active,
	}
}

func (s *Server) listStaff(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListStaff(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, st := range list {
		out = append(out, s.staffJSON(r.Context(), st))
	}
	writeJSON(w, http.StatusOK, map[string]any{"staff": out})
}

func (s *Server) createStaff(w http.ResponseWriter, r *http.Request) {
	var req staffReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	req.Login = auth.NormalizeLogin(req.Login)
	if req.Login == "" || strings.TrimSpace(req.Name) == "" || req.Password == "" || !auth.ValidRole(req.Role) {
		writeError(w, http.StatusUnprocessableEntity, loyalty.CodeInvalidRequest, "login, name, password и role обязательны")
		return
	}
	if req.StoreID == "" {
		req.StoreID = store.DefaultStoreID()
	}
	if _, err := s.Store.GetStore(r.Context(), req.StoreID); store.IsNoRows(err) {
		writeError(w, http.StatusUnprocessableEntity, loyalty.CodeInvalidRequest, "Точка не найдена")
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	ph, err := auth.HashSecret(req.Password)
	if err != nil {
		writeErr(w, err)
		return
	}
	pin, err := auth.HashSecret(req.PIN)
	if err != nil {
		writeErr(w, err)
		return
	}
	st := store.Staff{
		ID:           uuid.NewString(),
		StoreID:      req.StoreID,
		Login:        req.Login,
		Name:         strings.TrimSpace(req.Name),
		PasswordHash: ph,
		PINHash:      pin,
		Role:         req.Role,
		Active:       true,
		CreatedAt:    time.Now().UTC(),
	}
	if req.Active != nil {
		st.Active = *req.Active
	}
	if err := s.Store.CreateStaff(r.Context(), st); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.staffJSON(r.Context(), st))
}

func (s *Server) patchStaff(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	st, err := s.Store.GetStaffByID(r.Context(), id)
	if store.IsNoRows(err) {
		writeError(w, http.StatusNotFound, loyalty.CodeInvalidRequest, "Сотрудник не найден")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if st.DeletedAt != nil {
		writeError(w, http.StatusNotFound, loyalty.CodeInvalidRequest, "Сотрудник не найден")
		return
	}
	var req staffReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.Name != "" {
		st.Name = req.Name
	}
	if req.Login != "" {
		st.Login = auth.NormalizeLogin(req.Login)
	}
	if req.Role != "" {
		if !auth.ValidRole(req.Role) {
			writeError(w, http.StatusUnprocessableEntity, loyalty.CodeInvalidRequest, "Неизвестная роль")
			return
		}
		st.Role = req.Role
	}
	if req.StoreID != "" {
		if _, err := s.Store.GetStore(r.Context(), req.StoreID); store.IsNoRows(err) {
			writeError(w, http.StatusUnprocessableEntity, loyalty.CodeInvalidRequest, "Точка не найдена")
			return
		} else if err != nil {
			writeErr(w, err)
			return
		}
		st.StoreID = req.StoreID
	}
	if req.Password != "" {
		h, err := auth.HashSecret(req.Password)
		if err != nil {
			writeErr(w, err)
			return
		}
		st.PasswordHash = h
	}
	if req.PIN != "" {
		h, err := auth.HashSecret(req.PIN)
		if err != nil {
			writeErr(w, err)
			return
		}
		st.PINHash = h
	}
	if req.Active != nil {
		st.Active = *req.Active
		if !st.Active {
			_ = s.Store.RevokeStaffSessions(r.Context(), st.ID)
		}
	}
	if err := s.Store.UpdateStaff(r.Context(), st); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.staffJSON(r.Context(), st))
}

func (s *Server) deleteStaff(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	actor := staffFrom(r)
	if actor.ID == id {
		writeError(w, http.StatusUnprocessableEntity, loyalty.CodeInvalidRequest, "Нельзя удалить свою учётную запись")
		return
	}
	st, err := s.Store.GetStaffByID(r.Context(), id)
	if store.IsNoRows(err) || (err == nil && st.DeletedAt != nil) {
		writeError(w, http.StatusNotFound, loyalty.CodeInvalidRequest, "Сотрудник не найден")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.Store.SoftDeleteStaff(r.Context(), id, time.Now().UTC()); err != nil {
		writeErr(w, err)
		return
	}
	_ = s.Store.RevokeStaffSessions(r.Context(), id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listStores(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListStores(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stores": list})
}

func (s *Server) createStore(w http.ResponseWriter, r *http.Request) {
	var req storeReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusUnprocessableEntity, loyalty.CodeInvalidRequest, "name обязателен")
		return
	}
	row := store.StoreRow{ID: uuid.NewString(), Name: req.Name, Address: req.Address, CreatedAt: time.Now().UTC()}
	if err := s.Store.CreateStore(r.Context(), row); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) patchStore(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := s.Store.GetStore(r.Context(), id)
	if store.IsNoRows(err) {
		writeError(w, http.StatusNotFound, loyalty.CodeInvalidRequest, "Точка не найдена")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	var req storeReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.Name != "" {
		row.Name = req.Name
	}
	if req.Address != "" {
		row.Address = req.Address
	}
	if err := s.Store.UpdateStore(r.Context(), row); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) deleteStore(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.Store.DeleteStore(r.Context(), id); store.IsNoRows(err) {
		writeError(w, http.StatusNotFound, loyalty.CodeInvalidRequest, "Точка не найдена")
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	m, err := s.Store.SettingsMap(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var m map[string]string
	if err := decodeJSON(r, &m); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.Store.PutSettings(r.Context(), m); err != nil {
		writeErr(w, err)
		return
	}
	s.getSettings(w, r)
}

func customerJSON(c store.Customer) map[string]any {
	return map[string]any{
		"id":      c.ID,
		"barcode": c.Barcode,
		"name":    c.DisplayName,
		"phone":   c.Phone,
		"points":  c.Points,
		"blocked": c.Blocked,
		"deleted": c.DeletedAt != nil,
	}
}

func (s *Server) listCustomers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	status := r.URL.Query().Get("status")
	list, err := s.Store.SearchCustomers(r.Context(), q, status, 100)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		out = append(out, customerJSON(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"customers": out})
}

func (s *Server) deleteCustomer(w http.ResponseWriter, r *http.Request) {
	s.setCustomerDeleted(w, r, true)
}

func (s *Server) restoreCustomer(w http.ResponseWriter, r *http.Request) {
	s.setCustomerDeleted(w, r, false)
}

func (s *Server) setCustomerDeleted(w http.ResponseWriter, r *http.Request, deleted bool) {
	id := chi.URLParam(r, "id")
	c, err := s.Store.GetCustomerByID(r.Context(), id)
	if store.IsNoRows(err) {
		writeError(w, http.StatusNotFound, loyalty.CodeCustomerNotFound, "Клиент не найден")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if (c.DeletedAt != nil) == deleted {
		writeJSON(w, http.StatusOK, customerJSON(c))
		return
	}
	if err := s.Store.SetCustomerDeleted(r.Context(), id, deleted); err != nil {
		writeErr(w, err)
		return
	}
	c, err = s.Store.GetCustomerByID(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, customerJSON(c))
}

func (s *Server) blockCustomer(w http.ResponseWriter, r *http.Request) {
	s.setBlocked(w, r, true)
}

func (s *Server) unblockCustomer(w http.ResponseWriter, r *http.Request) {
	s.setBlocked(w, r, false)
}

func (s *Server) setBlocked(w http.ResponseWriter, r *http.Request, blocked bool) {
	id := chi.URLParam(r, "id")
	c, err := s.Store.GetCustomerByID(r.Context(), id)
	if store.IsNoRows(err) {
		writeError(w, http.StatusNotFound, loyalty.CodeCustomerNotFound, "Клиент не найден")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if c.DeletedAt != nil {
		writeError(w, http.StatusUnprocessableEntity, loyalty.CodeCustomerDeleted, "Карта удалена")
		return
	}
	if err := s.Store.SetCustomerBlocked(r.Context(), id, blocked); store.IsNoRows(err) {
		writeError(w, http.StatusNotFound, loyalty.CodeCustomerNotFound, "Клиент не найден")
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "blocked": blocked})
}
