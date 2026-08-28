package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

func main() {
	migrateOnly := flag.Bool("migrate-only", false, "apply migrations and exit")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := bootstrap(ctx, st, cfg, log); err != nil {
		log.Error("bootstrap", "err", err)
		os.Exit(1)
	}
	if *migrateOnly {
		log.Info("migrations applied")
		return
	}

	ap := apple.New(cfg, log)
	goog := googlew.New(cfg, log)
	wlt := &wallet.Composite{Log: log, Store: st, Apple: ap, Google: goog}
	q := jobs.New(st, wlt, log)
	go q.Run(ctx)

	loy := &loyalty.Service{Store: st, Jobs: q, IssuerID: cfg.GoogleIssuerID}
	srv, err := httpapi.New(cfg, log, st, loy, wlt, q, ap, goog)
	if err != nil {
		log.Error("http", "err", err)
		os.Exit(1)
	}

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr, "db", cfg.DatabaseURL)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("listen", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	shut, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	_ = httpSrv.Shutdown(shut)
	log.Info("stopped")
}

func bootstrap(ctx context.Context, st *store.Store, cfg config.Config, log *slog.Logger) error {
	if cfg.AdminBootstrapLogin == "" || cfg.AdminBootstrapPassword == "" {
		return nil
	}
	if _, err := st.GetStaffByLogin(ctx, cfg.AdminBootstrapLogin); err == nil {
		return nil
	} else if !store.IsNoRows(err) {
		return err
	}
	hash, err := auth.HashSecret(cfg.AdminBootstrapPassword)
	if err != nil {
		return err
	}
	admin := store.Staff{
		ID:           store.NewID(),
		StoreID:      store.DefaultStoreID(),
		Login:        cfg.AdminBootstrapLogin,
		Name:         cfg.AdminBootstrapName,
		PasswordHash: hash,
		Role:         auth.RoleAdmin,
		Active:       true,
		CreatedAt:    time.Now().UTC(),
	}
	if err := st.CreateStaff(ctx, admin); err != nil {
		return err
	}
	log.Info("bootstrap admin created", "login", admin.Login)

	if cfg.CashierBootstrapLogin != "" && cfg.CashierBootstrapPassword != "" {
		if _, err := st.GetStaffByLogin(ctx, cfg.CashierBootstrapLogin); store.IsNoRows(err) {
			ch, err := auth.HashSecret(cfg.CashierBootstrapPassword)
			if err != nil {
				return err
			}
			c := store.Staff{
				ID:           store.NewID(),
				StoreID:      store.DefaultStoreID(),
				Login:        cfg.CashierBootstrapLogin,
				Name:         cfg.CashierBootstrapName,
				PasswordHash: ch,
				Role:         auth.RoleCashier,
				Active:       true,
				CreatedAt:    time.Now().UTC(),
			}
			if err := st.CreateStaff(ctx, c); err != nil {
				return err
			}
			log.Info("bootstrap cashier created", "login", c.Login)
		} else if err != nil {
			return err
		}
	}
	return nil
}
