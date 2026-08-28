package jobs

import (
	"context"
	"log/slog"
	"time"

	"merch/backend/internal/store"
	"merch/backend/internal/wallet"
)

// Queue persists wallet updates and drains them in a background goroutine
// so cashier commit never waits on Apple/Google.
type Queue struct {
	Store   *store.Store
	Updater wallet.Updater
	Log     *slog.Logger
	wake    chan struct{}
}

func New(st *store.Store, u wallet.Updater, log *slog.Logger) *Queue {
	return &Queue{
		Store:   st,
		Updater: u,
		Log:     log,
		wake:    make(chan struct{}, 1),
	}
}

func (q *Queue) Enqueue(ctx context.Context, customerID string) {
	if q == nil || q.Store == nil {
		return
	}
	if err := q.Store.EnqueueWalletJob(ctx, customerID); err != nil {
		q.log().Error("enqueue wallet job", "err", err)
		return
	}
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *Queue) Run(ctx context.Context) {
	t := time.NewTicker(400 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-q.wake:
			q.drain(ctx)
		case <-t.C:
			q.drain(ctx)
		}
	}
}

func (q *Queue) drain(ctx context.Context) {
	jobs, err := q.Store.ClaimWalletJobs(ctx, 50)
	if err != nil {
		q.log().Error("claim wallet jobs", "err", err)
		return
	}
	seen := map[string]struct{}{}
	for _, j := range jobs {
		if _, ok := seen[j.CustomerID]; ok {
			continue
		}
		seen[j.CustomerID] = struct{}{}
		if q.Updater != nil {
			if err := q.Updater.UpdateWallet(ctx, j.CustomerID); err != nil {
				q.log().Error("wallet update", "err", err, "customer_id", j.CustomerID)
				_ = q.Store.FailWalletJob(ctx, j.ID, err.Error())
				continue
			}
		}
		if err := q.Store.CompleteWalletJobsForCustomer(ctx, j.CustomerID); err != nil {
			q.log().Error("complete wallet jobs", "err", err)
		}
	}
}

func (q *Queue) log() *slog.Logger {
	if q.Log != nil {
		return q.Log
	}
	return slog.Default()
}
