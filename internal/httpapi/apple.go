package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"merch/backend/internal/loyalty"
	"merch/backend/internal/store"
)

type appleRegisterBody struct {
	PushToken string `json:"pushToken"`
}

func (s *Server) appleAuth(r *http.Request, serial string) error {
	tok := applePassToken(r.Header.Get("Authorization"))
	if tok == "" {
		return loyalty.Err(loyalty.CodeUnauthorized, "Нужен ApplePass токен")
	}
	_, err := s.Store.GetCustomerByAppleToken(r.Context(), tok, serial)
	if store.IsNoRows(err) {
		return loyalty.Err(loyalty.CodeUnauthorized, "Недействительный ApplePass токен")
	}
	return err
}

func (s *Server) appleRegister(w http.ResponseWriter, r *http.Request) {
	serial := chi.URLParam(r, "serial")
	device := chi.URLParam(r, "deviceLibraryId")
	passType := chi.URLParam(r, "passType")
	if passType != s.Cfg.ApplePassTypeID {
		writeError(w, http.StatusNotFound, loyalty.CodeCustomerNotFound, "Неизвестный pass type")
		return
	}
	if err := s.appleAuth(r, serial); err != nil {
		writeErr(w, err)
		return
	}
	var body appleRegisterBody
	if err := decodeJSON(r, &body); err != nil || body.PushToken == "" {
		writeError(w, http.StatusUnprocessableEntity, loyalty.CodeInvalidRequest, "pushToken обязателен")
		return
	}
	if err := s.Store.RegisterAppleDevice(r.Context(), serial, device, body.PushToken); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) appleUnregister(w http.ResponseWriter, r *http.Request) {
	serial := chi.URLParam(r, "serial")
	device := chi.URLParam(r, "deviceLibraryId")
	if err := s.appleAuth(r, serial); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.Store.UnregisterAppleDevice(r.Context(), serial, device); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) appleUpdated(w http.ResponseWriter, r *http.Request) {
	device := chi.URLParam(r, "deviceLibraryId")
	passType := chi.URLParam(r, "passType")
	if passType != s.Cfg.ApplePassTypeID {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var since time.Time
	if q := r.URL.Query().Get("passesUpdatedSince"); q != "" {
		if n, err := strconv.ParseInt(q, 10, 64); err == nil {
			since = time.Unix(n, 0).UTC()
		} else if t, err := time.Parse(time.RFC3339, q); err == nil {
			since = t
		}
	}
	serials, last, err := s.Store.ListUpdatedSerials(r.Context(), device, since)
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(serials) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"lastUpdated":   strconv.FormatInt(last.Unix(), 10),
		"serialNumbers": serials,
	})
}

func (s *Server) appleGetPass(w http.ResponseWriter, r *http.Request) {
	serial := chi.URLParam(r, "serial")
	if err := s.appleAuth(r, serial); err != nil {
		writeErr(w, err)
		return
	}
	c, err := s.Store.GetCustomerByID(r.Context(), serial)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.writePKPass(w, r, c)
}

func (s *Server) appleLog(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	var payload map[string]any
	_ = json.Unmarshal(b, &payload)
	if logs, ok := payload["logs"]; ok {
		s.Log.Info("passkit log", "entries", logs)
	} else if len(b) > 0 {
		s.Log.Info("passkit log", "raw_len", len(b))
	}
	w.WriteHeader(http.StatusOK)
}
