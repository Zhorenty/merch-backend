package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Tx struct {
	s  *Store
	tx *sql.Tx
}

func (s *Store) InTx(ctx context.Context, fn func(*Tx) error) error {
	tx, err := s.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(&Tx{s: s, tx: tx}); err != nil {
		return err
	}
	return tx.Commit()
}

func (t *Tx) Q(q string) string { return t.s.Q(q) }

func (t *Tx) LockCustomer(ctx context.Context, id string) (Customer, error) {
	if err := t.s.lockCustomer(ctx, t.tx, id); err != nil {
		return Customer{}, err
	}
	return t.s.queryCustomer(ctx, t.tx, `SELECT `+customerCols+` FROM customers WHERE id = ?`, id)
}

func (t *Tx) GetReceipt(ctx context.Context, id string) (Receipt, error) {
	return t.s.getReceipt(ctx, t.tx, id)
}

func (t *Tx) InsertReceipt(ctx context.Context, r Receipt) error {
	_, err := t.tx.ExecContext(ctx, t.Q(`
		INSERT INTO receipts (id, store_id, staff_id, customer_id, amount_rub, redeem_points, earn_points, status, points_after, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		r.ID, r.StoreID, r.StaffID, r.CustomerID, r.AmountRub, r.RedeemPoints, r.EarnPoints, r.Status, r.PointsAfter, r.CreatedAt,
	)
	return err
}

func (t *Tx) MarkRefunded(ctx context.Context, id string, pointsAfter int, at time.Time) error {
	_, err := t.tx.ExecContext(ctx, t.Q(`UPDATE receipts SET status = ?, points_after_refund = ?, refunded_at = ? WHERE id = ?`),
		ReceiptRefunded, pointsAfter, at, id)
	return err
}

func (t *Tx) SetPoints(ctx context.Context, customerID string, points int, at time.Time) error {
	_, err := t.tx.ExecContext(ctx, t.Q(`UPDATE customers SET points = ?, updated_at = ? WHERE id = ?`), points, at, customerID)
	return err
}

func (t *Tx) InsertLedger(ctx context.Context, customerID, receiptID string, delta int, reason, actorID string, at time.Time) error {
	var rec any
	if receiptID == "" {
		rec = nil
	} else {
		rec = receiptID
	}
	var actor any
	if actorID == "" {
		actor = nil
	} else {
		actor = actorID
	}
	_, err := t.tx.ExecContext(ctx, t.Q(`
		INSERT INTO ledger (id, customer_id, receipt_id, delta, reason, actor_staff_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`),
		uuid.NewString(), customerID, rec, delta, reason, actor, at,
	)
	return err
}

func (s *Store) GetReceipt(ctx context.Context, id string) (Receipt, error) {
	return s.getReceipt(ctx, s.DB, id)
}

func (s *Store) getReceipt(ctx context.Context, q querier, id string) (Receipt, error) {
	row := q.QueryRowContext(ctx, s.Q(`SELECT `+receiptCols+` FROM receipts WHERE id = ?`), id)
	return scanReceipt(row)
}

func scanReceipt(sc scanner) (Receipt, error) {
	var r Receipt
	err := sc.Scan(
		&r.ID, &r.StoreID, &r.StaffID, &r.CustomerID, &r.AmountRub, &r.RedeemPoints, &r.EarnPoints,
		&r.Status, &r.PointsAfter, &r.PointsAfterRefund, &r.CreatedAt, &r.RefundedAt,
	)
	return r, err
}

const receiptCols = `id, store_id, staff_id, customer_id, amount_rub, redeem_points, earn_points, status, points_after, points_after_refund, created_at, refunded_at`
