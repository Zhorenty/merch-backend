package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"merch/backend/internal/auth"
	"merch/backend/internal/store"
)

func (s *Server) recordActivity(ctx context.Context, actor store.Staff, storeID, kind, title, detail string) {
	name := strings.TrimSpace(actor.Name)
	if actor.ID == "" && name == "" {
		name = "Клиент"
	}
	err := s.Store.InsertActivity(ctx, store.Activity{
		ID:           store.NewID(),
		CreatedAt:    time.Now().UTC(),
		ActorStaffID: actor.ID,
		ActorName:    name,
		StoreID:      storeID,
		Kind:         kind,
		Title:        title,
		Detail:       detail,
	})
	if err != nil && s.Log != nil {
		s.Log.Warn("activity", "err", err)
	}
}

func (s *Server) getCashierActivity(w http.ResponseWriter, r *http.Request) {
	st := staffFrom(r)
	s.writeActivity(w, r, false, st.ID, st.StoreID)
}

func (s *Server) getAdminActivity(w http.ResponseWriter, r *http.Request) {
	s.writeActivity(w, r, true, "", "")
}

func (s *Server) writeActivity(w http.ResponseWriter, r *http.Request, all bool, staffID, storeID string) {
	list, err := s.Store.ListActivity(r.Context(), all, staffID, storeID, 80)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, a := range list {
		out = append(out, map[string]any{
			"id":         a.ID,
			"created_at": a.CreatedAt.UTC().Format(time.RFC3339),
			"actor_name": a.ActorName,
			"kind":       a.Kind,
			"title":      a.Title,
			"detail":     a.Detail,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"activity": out})
}

func customerLabel(c store.Customer) string {
	name := strings.TrimSpace(c.DisplayName)
	if name == "" {
		name = c.Barcode
	}
	if c.Phone != "" {
		return name + " · " + c.Phone
	}
	return name
}

func roleLabel(role string) string {
	if role == auth.RoleAdmin {
		return "Админ"
	}
	return "Кассир"
}

func receiptActivity(amount, redeem, earn int, barcode string) (title, detail string) {
	switch {
	case redeem > 0 && earn > 0:
		title = "Чек: списание и начисление"
	case redeem > 0:
		title = "Списание баллов"
	case earn > 0:
		title = "Начисление баллов"
	default:
		title = "Чек"
	}
	detail = fmt.Sprintf("%d ₽ · списано %d · начислено %d · %s", amount, redeem, earn, barcode)
	return title, detail
}

func pointsActivity(delta int, who, reason string) (title, detail string) {
	title = "Начисление баллов"
	if delta < 0 {
		title = "Списание баллов"
	}
	detail = fmt.Sprintf("%+d · %s", delta, who)
	if strings.TrimSpace(reason) != "" {
		detail += " · " + strings.TrimSpace(reason)
	}
	return title, detail
}
