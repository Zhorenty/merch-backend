package store

import (
	"context"
	"database/sql"
	"time"
)

func scanStaff(sc scanner) (Staff, error) {
	var st Staff
	var active int
	err := sc.Scan(&st.ID, &st.StoreID, &st.Login, &st.Name, &st.PasswordHash, &st.PINHash, &st.Role, &active, &st.CreatedAt)
	st.Active = active != 0
	return st, err
}

const staffCols = `id, store_id, login, name, password_hash, pin_hash, role, active, created_at`

func (s *Store) GetStaffByLogin(ctx context.Context, login string) (Staff, error) {
	row := s.DB.QueryRowContext(ctx, s.Q(`SELECT `+staffCols+` FROM staff WHERE login = ?`), login)
	return scanStaff(row)
}

func (s *Store) GetStaffByID(ctx context.Context, id string) (Staff, error) {
	row := s.DB.QueryRowContext(ctx, s.Q(`SELECT `+staffCols+` FROM staff WHERE id = ?`), id)
	return scanStaff(row)
}

func (s *Store) ListStaff(ctx context.Context) ([]Staff, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+staffCols+` FROM staff ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Staff
	for rows.Next() {
		st, err := scanStaff(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *Store) CreateStaff(ctx context.Context, st Staff) error {
	_, err := s.DB.ExecContext(ctx, s.Q(`
		INSERT INTO staff (id, store_id, login, name, password_hash, pin_hash, role, active, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		st.ID, st.StoreID, st.Login, st.Name, st.PasswordHash, st.PINHash, st.Role, boolToInt(st.Active), st.CreatedAt,
	)
	return err
}

func (s *Store) UpdateStaff(ctx context.Context, st Staff) error {
	res, err := s.DB.ExecContext(ctx, s.Q(`
		UPDATE staff SET store_id = ?, login = ?, name = ?, password_hash = ?, pin_hash = ?, role = ?, active = ?
		WHERE id = ?`),
		st.StoreID, st.Login, st.Name, st.PasswordHash, st.PINHash, st.Role, boolToInt(st.Active), st.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListStores(ctx context.Context) ([]StoreRow, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, address, created_at FROM stores ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoreRow
	for rows.Next() {
		var r StoreRow
		if err := rows.Scan(&r.ID, &r.Name, &r.Address, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetStore(ctx context.Context, id string) (StoreRow, error) {
	var r StoreRow
	err := s.DB.QueryRowContext(ctx, s.Q(`SELECT id, name, address, created_at FROM stores WHERE id = ?`), id).
		Scan(&r.ID, &r.Name, &r.Address, &r.CreatedAt)
	return r, err
}

func (s *Store) CreateStore(ctx context.Context, r StoreRow) error {
	_, err := s.DB.ExecContext(ctx, s.Q(`INSERT INTO stores (id, name, address, created_at) VALUES (?, ?, ?, ?)`),
		r.ID, r.Name, r.Address, r.CreatedAt)
	return err
}

func (s *Store) UpdateStore(ctx context.Context, r StoreRow) error {
	res, err := s.DB.ExecContext(ctx, s.Q(`UPDATE stores SET name = ?, address = ? WHERE id = ?`), r.Name, r.Address, r.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteStore(ctx context.Context, id string) error {
	res, err := s.DB.ExecContext(ctx, s.Q(`DELETE FROM stores WHERE id = ?`), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) CreateSession(ctx context.Context, jti, staffID, kind string, exp time.Time) error {
	_, err := s.DB.ExecContext(ctx, s.Q(`INSERT INTO staff_sessions (jti, staff_id, kind, expires_at, revoked) VALUES (?, ?, ?, ?, 0)`),
		jti, staffID, kind, exp.UTC())
	return err
}

func (s *Store) RevokeSession(ctx context.Context, jti string) error {
	_, err := s.DB.ExecContext(ctx, s.Q(`UPDATE staff_sessions SET revoked = 1 WHERE jti = ?`), jti)
	return err
}

func (s *Store) RevokeStaffSessions(ctx context.Context, staffID string) error {
	_, err := s.DB.ExecContext(ctx, s.Q(`UPDATE staff_sessions SET revoked = 1 WHERE staff_id = ?`), staffID)
	return err
}

func (s *Store) SessionValid(ctx context.Context, jti string) (bool, error) {
	var revoked int
	var exp time.Time
	err := s.DB.QueryRowContext(ctx, s.Q(`SELECT revoked, expires_at FROM staff_sessions WHERE jti = ?`), jti).Scan(&revoked, &exp)
	if errorsIsNoRows(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if revoked != 0 {
		return false, nil
	}
	return time.Now().Before(exp), nil
}

func errorsIsNoRows(err error) bool {
	return err == sql.ErrNoRows
}

func DefaultStoreID() string {
	return "00000000-0000-4000-8000-000000000001"
}
