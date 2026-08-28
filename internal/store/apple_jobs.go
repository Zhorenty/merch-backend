package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

func (s *Store) RegisterAppleDevice(ctx context.Context, customerID, deviceLibraryID, pushToken string) error {
	_, err := s.DB.ExecContext(ctx, s.Q(`
		INSERT INTO apple_devices (id, customer_id, device_library_id, push_token, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (device_library_id, customer_id) DO UPDATE SET push_token = excluded.push_token`),
		uuid.NewString(), customerID, deviceLibraryID, pushToken, time.Now().UTC(),
	)
	return err
}

func (s *Store) UnregisterAppleDevice(ctx context.Context, customerID, deviceLibraryID string) error {
	_, err := s.DB.ExecContext(ctx, s.Q(`DELETE FROM apple_devices WHERE customer_id = ? AND device_library_id = ?`),
		customerID, deviceLibraryID)
	return err
}

func (s *Store) ListAppleDevices(ctx context.Context, customerID string) ([]AppleDevice, error) {
	rows, err := s.DB.QueryContext(ctx, s.Q(`
		SELECT id, customer_id, device_library_id, push_token, created_at
		FROM apple_devices WHERE customer_id = ?`), customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AppleDevice
	for rows.Next() {
		var d AppleDevice
		if err := rows.Scan(&d.ID, &d.CustomerID, &d.DeviceLibraryID, &d.PushToken, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ListUpdatedSerials(ctx context.Context, deviceLibraryID string, since time.Time) ([]string, time.Time, error) {
	q := `
		SELECT c.id, c.updated_at
		FROM apple_devices d
		JOIN customers c ON c.id = d.customer_id
		WHERE d.device_library_id = ?`
	args := []any{deviceLibraryID}
	if !since.IsZero() {
		q += ` AND c.updated_at > ?`
		args = append(args, since.UTC())
	}
	q += ` ORDER BY c.updated_at`
	rows, err := s.DB.QueryContext(ctx, s.Q(q), args...)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer rows.Close()
	var serials []string
	var last time.Time
	for rows.Next() {
		var id string
		var u time.Time
		if err := rows.Scan(&id, &u); err != nil {
			return nil, time.Time{}, err
		}
		serials = append(serials, id)
		if u.After(last) {
			last = u
		}
	}
	return serials, last, rows.Err()
}

func (s *Store) EnqueueWalletJob(ctx context.Context, customerID string) error {
	// Coalesce: skip if a pending job already exists for this customer.
	var pending int
	err := s.DB.QueryRowContext(ctx, s.Q(`
		SELECT COUNT(1) FROM wallet_jobs WHERE customer_id = ? AND processed_at IS NULL`), customerID).Scan(&pending)
	if err != nil {
		return err
	}
	if pending > 0 {
		return nil
	}
	_, err = s.DB.ExecContext(ctx, s.Q(`
		INSERT INTO wallet_jobs (id, customer_id, kind, attempts, created_at) VALUES (?, ?, 'wallet_update', 0, ?)`),
		uuid.NewString(), customerID, time.Now().UTC(),
	)
	return err
}

func (s *Store) ClaimWalletJobs(ctx context.Context, limit int) ([]WalletJob, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.DB.QueryContext(ctx, s.Q(`
		SELECT id, customer_id, kind, attempts, last_error, created_at, processed_at
		FROM wallet_jobs
		WHERE processed_at IS NULL
		ORDER BY created_at
		LIMIT ?`), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WalletJob
	for rows.Next() {
		var j WalletJob
		if err := rows.Scan(&j.ID, &j.CustomerID, &j.Kind, &j.Attempts, &j.LastError, &j.CreatedAt, &j.ProcessedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Store) CompleteWalletJobsForCustomer(ctx context.Context, customerID string) error {
	_, err := s.DB.ExecContext(ctx, s.Q(`UPDATE wallet_jobs SET processed_at = ? WHERE customer_id = ? AND processed_at IS NULL`),
		time.Now().UTC(), customerID)
	return err
}

func (s *Store) FailWalletJob(ctx context.Context, id, lastErr string) error {
	_, err := s.DB.ExecContext(ctx, s.Q(`UPDATE wallet_jobs SET attempts = attempts + 1, last_error = ? WHERE id = ?`), lastErr, id)
	return err
}

func (s *Store) TouchDedupe(ctx context.Context, key string) error {
	_, err := s.DB.ExecContext(ctx, s.Q(`INSERT INTO job_dedupe (key, created_at) VALUES (?, ?) ON CONFLICT(key) DO NOTHING`),
		key, time.Now().UTC())
	return err
}

func IsNoRows(err error) bool {
	return err == sql.ErrNoRows
}
