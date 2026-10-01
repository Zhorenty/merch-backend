package store

import (
	"context"
	"time"
)

type Activity struct {
	ID           string
	CreatedAt    time.Time
	ActorStaffID string
	ActorName    string
	StoreID      string
	Kind         string
	Title        string
	Detail       string
}

func (s *Store) InsertActivity(ctx context.Context, a Activity) error {
	_, err := s.DB.ExecContext(ctx, s.Q(`
		INSERT INTO activity (id, created_at, actor_staff_id, actor_name, store_id, kind, title, detail)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		a.ID, a.CreatedAt, a.ActorStaffID, a.ActorName, a.StoreID, a.Kind, a.Title, a.Detail,
	)
	return err
}

func (s *Store) ListActivity(ctx context.Context, all bool, staffID, storeID string, limit int) ([]Activity, error) {
	if limit <= 0 || limit > 100 {
		limit = 80
	}
	q := `SELECT id, created_at, actor_staff_id, actor_name, store_id, kind, title, detail FROM activity`
	args := []any{}
	if !all {
		q += ` WHERE actor_staff_id = ? OR store_id = ? OR store_id = ''`
		args = append(args, staffID, storeID)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, s.Q(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Activity
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.CreatedAt, &a.ActorStaffID, &a.ActorName, &a.StoreID, &a.Kind, &a.Title, &a.Detail); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
