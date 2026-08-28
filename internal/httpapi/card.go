package httpapi

import (
	"encoding/base64"
	"html/template"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/skip2/go-qrcode"
)

type cardPage struct {
	Name       string
	Barcode    string
	Points     int
	QRDataURI  template.URL
	AppleURL   string
	GoogleURL  string
	Platform   string
	TermsURL   string
	Support    string
	BackText   string
	ShowApple  bool
	ShowGoogle bool
	ShowWeb    bool
	EnrollJSON bool
}

func detectPlatform(r *http.Request) string {
	if p := r.URL.Query().Get("platform"); p != "" {
		return p
	}
	ua := strings.ToLower(r.UserAgent())
	if strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad") || strings.Contains(ua, "ipod") {
		return "apple"
	}
	huawei := strings.Contains(ua, "huawei") || strings.Contains(ua, "honor") || strings.Contains(ua, "harmony")
	if strings.Contains(ua, "android") && !huawei {
		return "google"
	}
	return "web"
}

func (s *Server) getCardAddLanding(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("merch_cid"); err == nil && c.Value != "" {
		if _, err := s.Store.GetCustomerByID(r.Context(), c.Value); err == nil {
			http.Redirect(w, r, "/card/add/"+c.Value, http.StatusFound)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.pages.ExecuteTemplate(w, "enroll.gohtml", map[string]any{
		"TermsURL": s.Cfg.TermsURL,
		"Support":  s.Cfg.SupportContact,
	})
}

func (s *Server) getCardAdd(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, err := s.loadCustomer(r, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	png, err := qrcode.Encode(c.Barcode, qrcode.Medium, 280)
	if err != nil {
		writeErr(w, err)
		return
	}
	googleURL := ""
	if s.Google != nil {
		googleURL, _ = s.Google.SaveURL(c)
	}
	p := detectPlatform(r)
	page := cardPage{
		Name:       c.DisplayName,
		Barcode:    c.Barcode,
		Points:     c.Points,
		QRDataURI:  template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(png)),
		AppleURL:   s.Cfg.APIBaseURL + "/public/passes/apple/" + c.ID + ".pkpass",
		GoogleURL:  googleURL,
		Platform:   p,
		TermsURL:   s.Cfg.TermsURL,
		Support:    s.Cfg.SupportContact,
		BackText:   "Баллы начисляются с покупок в MERCH и списываются на кассе. Карта — не платёжное средство. Правила может изменить магазин. Вопросы: " + s.Cfg.SupportContact + ".",
		ShowApple:  p == "apple",
		ShowGoogle: p == "google",
		ShowWeb:    p == "web" || p == "google" || p == "apple",
	}
	if p == "web" {
		page.ShowApple = false
		page.ShowGoogle = false
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.pages.ExecuteTemplate(w, "add.gohtml", page)
}
