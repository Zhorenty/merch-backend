package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

func scanCustomer(s scanner) (Customer, error) {
	var c Customer
	var blocked int
	err := s.Scan(
		&c.ID, &c.Barcode, &c.DisplayName, &c.Phone, &c.AppleAuthToken, &c.GoogleObjectID,
		&c.Points, &blocked, &c.CreatedAt, &c.UpdatedAt,
	)
	c.Blocked = blocked != 0
	return c, err
}

const customerCols = `id, barcode, display_name, phone, apple_auth_token, google_object_id, points, blocked, created_at, updated_at`

func (s *Store) GetCustomerByID(ctx context.Context, id string) (Customer, error) {
	return s.queryCustomer(ctx, s.DB, `SELECT `+customerCols+` FROM customers WHERE id = ?`, id)
}

func (s *Store) GetCustomerByBarcode(ctx context.Context, barcode string) (Customer, error) {
	return s.queryCustomer(ctx, s.DB, `SELECT `+customerCols+` FROM customers WHERE barcode = ?`, barcode)
}

func (s *Store) GetCustomerByPhone(ctx context.Context, phone string) (Customer, error) {
	if phone == "" {
		return Customer{}, sql.ErrNoRows
	}
	return s.queryCustomer(ctx, s.DB, `SELECT `+customerCols+` FROM customers WHERE phone = ?`, phone)
}

func (s *Store) GetCustomerByAppleToken(ctx context.Context, token, serial string) (Customer, error) {
	return s.queryCustomer(ctx, s.DB, `SELECT `+customerCols+` FROM customers WHERE apple_auth_token = ? AND id = ?`, token, serial)
}

func (s *Store) queryCustomer(ctx context.Context, q querier, query string, args ...any) (Customer, error) {
	row := q.QueryRowContext(ctx, s.Q(query), args...)
	c, err := scanCustomer(row)
	if err != nil {
		return Customer{}, err
	}
	return c, nil
}

func (s *Store) CreateCustomer(ctx context.Context, c Customer) error {
	_, err := s.DB.ExecContext(ctx, s.Q(`
		INSERT INTO customers (id, barcode, display_name, phone, apple_auth_token, google_object_id, points, blocked, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, 0, ?, ?)`),
		c.ID, c.Barcode, c.DisplayName, c.Phone, c.AppleAuthToken, c.GoogleObjectID, c.CreatedAt, c.UpdatedAt,
	)
	return err
}

func (s *Store) SetCustomerBlocked(ctx context.Context, id string, blocked bool) error {
	res, err := s.DB.ExecContext(ctx, s.Q(`UPDATE customers SET blocked = ?, updated_at = ? WHERE id = ?`),
		boolToInt(blocked), time.Now().UTC(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SearchCustomers(ctx context.Context, qstr string, limit int) ([]Customer, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	qstr = strings.TrimSpace(qstr)
	var rows *sql.Rows
	var err error
	if qstr == "" {
		rows, err = s.DB.QueryContext(ctx, s.Q(`SELECT `+customerCols+` FROM customers ORDER BY created_at DESC LIMIT ?`), limit)
	} else {
		like := "%" + qstr + "%"
		rows, err = s.DB.QueryContext(ctx, s.Q(`
			SELECT `+customerCols+` FROM customers
			WHERE barcode = ? OR phone = ? OR id = ? OR display_name LIKE ? OR barcode LIKE ? OR phone LIKE ?
			ORDER BY created_at DESC LIMIT ?`),
			qstr, qstr, qstr, like, like, like, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Customer
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) PutSettings(ctx context.Context, kv map[string]string) error {
	tx, err := s.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for k, v := range kv {
		if _, err := tx.ExecContext(ctx, s.Q(`
			INSERT INTO loyalty_settings (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`), k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SettingsMap(ctx context.Context) (map[string]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value FROM loyalty_settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, rows.Err()
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type scanner interface {
	Scan(dest ...any) error
}

var ErrNotFound = errors.New("not found")

func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrNotFound)
}

func NewID() string { return uuid.NewString() }
