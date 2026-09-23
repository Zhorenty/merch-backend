package httpapi

import (
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"merch/backend/internal/config"
	"merch/backend/internal/jobs"
	"merch/backend/internal/loyalty"
	"merch/backend/internal/store"
	"merch/backend/internal/wallet"
	"merch/backend/internal/wallet/apple"
	googlew "merch/backend/internal/wallet/google"
	"merch/backend/web"
)

type Server struct {
	Cfg     config.Config
	Log     *slog.Logger
	Store   *store.Store
	Loyalty *loyalty.Service
	Wallet  *wallet.Composite
	Jobs    *jobs.Queue
	Apple   *apple.Client
	Google  *googlew.Client
	pages   *template.Template
	enrollL *ipLimiter
	lookupL *ipLimiter
}

func New(cfg config.Config, log *slog.Logger, st *store.Store, loy *loyalty.Service, wlt *wallet.Composite, q *jobs.Queue, ap *apple.Client, goog *googlew.Client) (*Server, error) {
	sub, err := fs.Sub(web.Templates, "templates")
	if err != nil {
		return nil, err
	}
	tpl, err := template.New("").ParseFS(sub, "*.gohtml")
	if err != nil {
		return nil, err
	}
	return &Server{
		Cfg:     cfg,
		Log:     log,
		Store:   st,
		Loyalty: loy,
		Wallet:  wlt,
		Jobs:    q,
		Apple:   ap,
		Google:  goog,
		pages:   tpl,
		enrollL: newIPLimiter(10, 5),
		lookupL: newIPLimiter(60, 20),
	}, nil
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(maxBody(1 << 20))
	r.Use(withTimeout(15 * time.Second))
	r.Use(requestLog(s.Log))
	r.Use(cors(s.Cfg.CORSList()))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000")
			}
			next.ServeHTTP(w, r)
		})
	})

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/public", func(r chi.Router) {
		r.With(s.rateLimit(s.enrollL)).Post("/enroll", s.postPublicEnroll)
		r.Get("/passes/apple/{id}.pkpass", s.getApplePass)
		r.Get("/passes/apple/{id}", s.getApplePass)
	})

	r.Get("/loyalty-terms", s.getLoyaltyTerms)
	r.Get("/card/add", s.getCardAddLanding)
	r.Get("/card/add/{id}", s.getCardAdd)

	r.Route("/cashier", func(r chi.Router) {
		r.Post("/login", s.postCashierLogin)
		r.Get("/app-version", s.getAppVersion)
		r.Group(func(r chi.Router) {
			r.Use(s.requireStaff(authKindCashier(), s.Cfg.CashierJWTSecret))
			r.With(s.rateLimit(s.lookupL)).Post("/lookup", s.postLookup)
			r.Post("/quote-redeem", s.postQuote)
			r.Post("/commit", s.postCommit)
			r.Post("/refund", s.postRefund)
			r.Post("/enroll", s.postCashierEnroll)
		})
	})

	r.Route("/admin", func(r chi.Router) {
		r.Post("/login", s.postAdminLogin)
		r.Group(func(r chi.Router) {
			r.Use(s.requireStaff(authKindAdmin(), s.Cfg.AdminJWTSecret))
			r.Post("/adjust", s.postAdjust)
			r.Get("/staff", s.listStaff)
			r.Post("/staff", s.createStaff)
			r.Patch("/staff/{id}", s.patchStaff)
			r.Get("/stores", s.listStores)
			r.Post("/stores", s.createStore)
			r.Patch("/stores/{id}", s.patchStore)
			r.Delete("/stores/{id}", s.deleteStore)
			r.Get("/loyalty-settings", s.getSettings)
			r.Put("/loyalty-settings", s.putSettings)
			r.Get("/customers", s.listCustomers)
			r.Post("/customers/{id}/block", s.blockCustomer)
			r.Post("/customers/{id}/unblock", s.unblockCustomer)
		})
	})

	r.Route("/passes", func(r chi.Router) {
		r.Post("/v1/devices/{deviceLibraryId}/registrations/{passType}/{serial}", s.appleRegister)
		r.Delete("/v1/devices/{deviceLibraryId}/registrations/{passType}/{serial}", s.appleUnregister)
		r.Get("/v1/devices/{deviceLibraryId}/registrations/{passType}", s.appleUpdated)
		r.Get("/v1/passes/{passType}/{serial}", s.appleGetPass)
		r.Post("/v1/log", s.appleLog)
	})

	return r
}

func authKindCashier() string { return "cashier" }
func authKindAdmin() string   { return "admin" }
