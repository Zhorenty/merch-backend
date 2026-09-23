package wallet

import (
	"context"
	"log/slog"

	"merch/backend/internal/store"
)

type Updater interface {
	UpdateWallet(ctx context.Context, customerID string) error
}

type Composite struct {
	Log    *slog.Logger
	Store  *store.Store
	Apple  Apple
	Google Google
}

type Apple interface {
	Enabled() bool
	BuildPKPass(ctx context.Context, c store.Customer, earnPercent int) ([]byte, error)
	PushUpdate(ctx context.Context, tokens []string) error
}

type Google interface {
	Enabled() bool
	SaveURL(c store.Customer) (string, error)
	PatchPoints(ctx context.Context, objectID string, points int) error
}

func (c *Composite) UpdateWallet(ctx context.Context, customerID string) error {
	cust, err := c.Store.GetCustomerByID(ctx, customerID)
	if err != nil {
		return err
	}
	if c.Apple != nil && c.Apple.Enabled() {
		devs, err := c.Store.ListAppleDevices(ctx, customerID)
		if err != nil {
			c.log().Error("list apple devices", "err", err)
		} else {
			tokens := make([]string, 0, len(devs))
			for _, d := range devs {
				if d.PushToken != "" {
					tokens = append(tokens, d.PushToken)
				}
			}
			if len(tokens) > 0 {
				if err := c.Apple.PushUpdate(ctx, tokens); err != nil {
					c.log().Error("apple push skipped", "err", err)
				}
			}
		}
	} else {
		c.log().Info("wallet update skipped", "provider", "apple", "customer_id", customerID)
	}
	if c.Google != nil && c.Google.Enabled() {
		if err := c.Google.PatchPoints(ctx, cust.GoogleObjectID, cust.Points); err != nil {
			c.log().Error("google patch skipped", "err", err)
		}
	} else {
		c.log().Info("wallet update skipped", "provider", "google", "customer_id", customerID)
	}
	return nil
}

func (c *Composite) log() *slog.Logger {
	if c.Log != nil {
		return c.Log
	}
	return slog.Default()
}
