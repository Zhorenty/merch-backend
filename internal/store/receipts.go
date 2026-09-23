package store

import (
	"context"
	"time"
)

// Moscow is UTC+3 year-round. The API image is distroless and has no zoneinfo,
// so the store calendar day is a fixed offset, not time.LoadLocation.
var moscow = time.FixedZone("MSK", 3*60*60)

// DayBounds is the store calendar day [start, end) that contains now.
func DayBounds(now time.Time) (start, end time.Time) {
	local := now.In(moscow)
	start = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, moscow)
	return start, start.Add(24 * time.Hour)
}

// ShiftReceipt is one row of the cashier shift list.
type ShiftReceipt struct {
	ID           string
	CreatedAt    time.Time
	Barcode      string
	Name         string
	AmountRub    int
	RedeemPoints int
	EarnPoints   int
	Status       string
	PointsAfter  int
}

// ListStoreReceipts returns receipts of one store in [from, until), newest first.
// Refunded rows expose the balance after the refund.
func (s *Store) ListStoreReceipts(ctx context.Context, storeID string, from, until time.Time) ([]ShiftReceipt, error) {
	rows, err := s.DB.QueryContext(ctx, s.Q(`
		SELECT r.id, r.created_at, c.barcode, c.display_name,
		       r.amount_rub, r.redeem_points, r.earn_points, r.status,
		       CASE WHEN r.status = ? THEN COALESCE(r.points_after_refund, r.points_after) ELSE r.points_after END
		FROM receipts r
		INNER JOIN customers c ON c.id = r.customer_id
		WHERE r.store_id = ?
		  AND r.created_at >= ?
		  AND r.created_at < ?
		  AND r.status IN (?, ?)
		ORDER BY r.created_at DESC, r.id DESC`),
		ReceiptRefunded, storeID, from.UTC(), until.UTC(), ReceiptCommitted, ReceiptRefunded,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ShiftReceipt, 0)
	for rows.Next() {
		var rec ShiftReceipt
		if err := rows.Scan(
			&rec.ID, &rec.CreatedAt, &rec.Barcode, &rec.Name,
			&rec.AmountRub, &rec.RedeemPoints, &rec.EarnPoints, &rec.Status, &rec.PointsAfter,
		); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
