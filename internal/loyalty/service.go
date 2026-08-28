package loyalty

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"merch/backend/internal/auth"
	"merch/backend/internal/barcode"
	"merch/backend/internal/store"
)

type Enqueuer interface {
	Enqueue(ctx context.Context, customerID string)
}

type Service struct {
	Store    *store.Store
	Jobs     Enqueuer
	IssuerID string
}

type EnrollInput struct {
	Name      string
	Phone     string
	CookieID  string
	GoogleOID string
}

type EnrollResult struct {
	Customer store.Customer
	Created  bool
}

type LookupResult struct {
	Customer  store.Customer
	Settings  Settings
	CanRedeem bool
}

type CommitInput struct {
	ReceiptID    string
	Barcode      string
	AmountRub    int
	RedeemPoints int
	StoreID      string
	StaffID      string
}

type CommitResult struct {
	ReceiptID        string `json:"receipt_id"`
	CustomerID       string `json:"customer_id"`
	Barcode          string `json:"barcode"`
	Points           int    `json:"points"`
	EarnPoints       int    `json:"earn_points"`
	RedeemPoints     int    `json:"redeem_points"`
	IdempotentReplay bool   `json:"idempotent_replay"`
}

type RefundResult struct {
	ReceiptID        string `json:"receipt_id"`
	CustomerID       string `json:"customer_id"`
	Points           int    `json:"points"`
	IdempotentReplay bool   `json:"idempotent_replay"`
}

func (s *Service) Enroll(ctx context.Context, in EnrollInput) (EnrollResult, error) {
	if in.CookieID != "" {
		c, err := s.Store.GetCustomerByID(ctx, in.CookieID)
		if err == nil {
			return EnrollResult{Customer: c, Created: false}, nil
		}
		if !store.IsNoRows(err) {
			return EnrollResult{}, err
		}
	}
	phone := NormalizePhone(in.Phone)
	if phone != "" {
		c, err := s.Store.GetCustomerByPhone(ctx, phone)
		if err == nil {
			return EnrollResult{Customer: c, Created: false}, nil
		}
		if !store.IsNoRows(err) {
			return EnrollResult{}, err
		}
	}

	id := uuid.NewString()
	code, err := barcode.New()
	if err != nil {
		return EnrollResult{}, err
	}
	token, err := auth.RandomToken(24)
	if err != nil {
		return EnrollResult{}, err
	}
	now := time.Now().UTC()
	oid := in.GoogleOID
	if oid == "" {
		issuer := s.IssuerID
		if issuer == "" {
			issuer = "merch"
		}
		oid = issuer + "." + id
	}
	c := store.Customer{
		ID:             id,
		Barcode:        code,
		DisplayName:    strings.TrimSpace(in.Name),
		Phone:          phone,
		AppleAuthToken: token,
		GoogleObjectID: oid,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.Store.CreateCustomer(ctx, c); err != nil {
		return EnrollResult{}, err
	}
	return EnrollResult{Customer: c, Created: true}, nil
}

func (s *Service) resolveCustomer(ctx context.Context, barcodeOrID string) (store.Customer, error) {
	raw := strings.TrimSpace(barcodeOrID)
	if raw == "" {
		return store.Customer{}, Err(CodeInvalidRequest, "Укажите barcode")
	}
	norm := barcode.Normalize(raw)
	c, err := s.Store.GetCustomerByBarcode(ctx, norm)
	if err == nil {
		return c, nil
	}
	if !store.IsNoRows(err) {
		return store.Customer{}, err
	}
	if barcode.LooksLikeCustomerID(raw) {
		c, err = s.Store.GetCustomerByID(ctx, raw)
		if err == nil {
			return c, nil
		}
		if !store.IsNoRows(err) {
			return store.Customer{}, err
		}
	}
	return store.Customer{}, Err(CodeCustomerNotFound, "Клиент не найден")
}

func (s *Service) settings(ctx context.Context) (Settings, error) {
	m, err := s.Store.SettingsMap(ctx)
	if err != nil {
		return Settings{}, err
	}
	return ParseSettings(m), nil
}

func (s *Service) Lookup(ctx context.Context, code string) (LookupResult, error) {
	c, err := s.resolveCustomer(ctx, code)
	if err != nil {
		return LookupResult{}, err
	}
	if c.Blocked {
		return LookupResult{}, Err(CodeCustomerBlocked, "Карта заблокирована")
	}
	st, err := s.settings(ctx)
	if err != nil {
		return LookupResult{}, err
	}
	return LookupResult{
		Customer:  c,
		Settings:  st,
		CanRedeem: c.Points >= st.RedeemMin,
	}, nil
}

func (s *Service) Quote(ctx context.Context, code string, amountRub, requested int) (Quote, error) {
	c, err := s.resolveCustomer(ctx, code)
	if err != nil {
		return Quote{}, err
	}
	if c.Blocked {
		return Quote{}, Err(CodeCustomerBlocked, "Карта заблокирована")
	}
	if amountRub < 0 || requested < 0 {
		return Quote{}, Err(CodeInvalidRequest, "Сумма и баллы не могут быть отрицательными")
	}
	st, err := s.settings(ctx)
	if err != nil {
		return Quote{}, err
	}
	return QuoteRedeem(c.Points, amountRub, requested, st), nil
}

func (s *Service) Commit(ctx context.Context, in CommitInput) (CommitResult, error) {
	if _, err := uuid.Parse(in.ReceiptID); err != nil {
		return CommitResult{}, Err(CodeInvalidRequest, "receipt_id должен быть UUID")
	}
	if in.AmountRub < 0 || in.RedeemPoints < 0 {
		return CommitResult{}, Err(CodeInvalidRequest, "Сумма и баллы не могут быть отрицательными")
	}
	c, err := s.resolveCustomer(ctx, in.Barcode)
	if err != nil {
		return CommitResult{}, err
	}
	if c.Blocked {
		return CommitResult{}, Err(CodeCustomerBlocked, "Карта заблокирована")
	}
	st, err := s.settings(ctx)
	if err != nil {
		return CommitResult{}, err
	}

	var result CommitResult
	err = s.Store.InTx(ctx, func(tx *store.Tx) error {
		existing, e := tx.GetReceipt(ctx, in.ReceiptID)
		if e == nil {
			if existing.CustomerID != c.ID || existing.AmountRub != in.AmountRub || existing.RedeemPoints != in.RedeemPoints {
				return Err(CodeDuplicateReceipt, "Чек уже проведён с другими данными")
			}
			result = CommitResult{
				ReceiptID:        existing.ID,
				CustomerID:       existing.CustomerID,
				Barcode:          c.Barcode,
				Points:           existing.PointsAfter,
				EarnPoints:       existing.EarnPoints,
				RedeemPoints:     existing.RedeemPoints,
				IdempotentReplay: true,
			}
			return nil
		}
		if e != sql.ErrNoRows {
			return e
		}

		locked, e := tx.LockCustomer(ctx, c.ID)
		if e != nil {
			return e
		}
		if locked.Blocked {
			return Err(CodeCustomerBlocked, "Карта заблокирована")
		}
		if err := CheckRedeem(locked.Points, in.AmountRub, in.RedeemPoints, st); err != nil {
			return err
		}
		earn := ComputeEarn(in.AmountRub, in.RedeemPoints, st)
		points := locked.Points - in.RedeemPoints + earn
		now := time.Now().UTC()
		rec := store.Receipt{
			ID:           in.ReceiptID,
			StoreID:      in.StoreID,
			StaffID:      in.StaffID,
			CustomerID:   locked.ID,
			AmountRub:    in.AmountRub,
			RedeemPoints: in.RedeemPoints,
			EarnPoints:   earn,
			Status:       store.ReceiptCommitted,
			PointsAfter:  points,
			CreatedAt:    now,
		}
		if e := tx.InsertReceipt(ctx, rec); e != nil {
			return e
		}
		if in.RedeemPoints != 0 {
			if e := tx.InsertLedger(ctx, locked.ID, rec.ID, -in.RedeemPoints, store.ReasonRedeem, in.StaffID, now); e != nil {
				return e
			}
		}
		if earn != 0 {
			if e := tx.InsertLedger(ctx, locked.ID, rec.ID, earn, store.ReasonEarn, in.StaffID, now); e != nil {
				return e
			}
		}
		if e := tx.SetPoints(ctx, locked.ID, points, now); e != nil {
			return e
		}
		result = CommitResult{
			ReceiptID:    rec.ID,
			CustomerID:   locked.ID,
			Barcode:      locked.Barcode,
			Points:       points,
			EarnPoints:   earn,
			RedeemPoints: in.RedeemPoints,
		}
		return nil
	})
	if err != nil {
		return CommitResult{}, err
	}
	if !result.IdempotentReplay {
		s.enqueue(ctx, result.CustomerID)
	}
	return result, nil
}

func (s *Service) Refund(ctx context.Context, receiptID, actorID string) (RefundResult, error) {
	if _, err := uuid.Parse(receiptID); err != nil {
		return RefundResult{}, Err(CodeInvalidRequest, "receipt_id должен быть UUID")
	}
	var out RefundResult
	err := s.Store.InTx(ctx, func(tx *store.Tx) error {
		rec, e := tx.GetReceipt(ctx, receiptID)
		if e == sql.ErrNoRows {
			return Err(CodeInvalidRequest, "Чек не найден")
		}
		if e != nil {
			return e
		}
		if rec.Status == store.ReceiptRefunded {
			pts := rec.PointsAfter
			if rec.PointsAfterRefund.Valid {
				pts = int(rec.PointsAfterRefund.Int64)
			}
			out = RefundResult{ReceiptID: rec.ID, CustomerID: rec.CustomerID, Points: pts, IdempotentReplay: true}
			return nil
		}
		locked, e := tx.LockCustomer(ctx, rec.CustomerID)
		if e != nil {
			return e
		}
		now := time.Now().UTC()
		points := locked.Points
		if rec.EarnPoints > 0 {
			rev := rec.EarnPoints
			if rev > points {
				rev = points
			}
			if rev > 0 {
				points -= rev
				if e := tx.InsertLedger(ctx, locked.ID, rec.ID, -rev, store.ReasonRefundEarn, actorID, now); e != nil {
					return e
				}
			}
		}
		if rec.RedeemPoints > 0 {
			points += rec.RedeemPoints
			if e := tx.InsertLedger(ctx, locked.ID, rec.ID, rec.RedeemPoints, store.ReasonRefundRedeem, actorID, now); e != nil {
				return e
			}
		}
		if e := tx.SetPoints(ctx, locked.ID, points, now); e != nil {
			return e
		}
		if e := tx.MarkRefunded(ctx, rec.ID, points, now); e != nil {
			return e
		}
		out = RefundResult{ReceiptID: rec.ID, CustomerID: rec.CustomerID, Points: points}
		return nil
	})
	if err != nil {
		return RefundResult{}, err
	}
	if !out.IdempotentReplay {
		s.enqueue(ctx, out.CustomerID)
	}
	return out, nil
}

func (s *Service) Adjust(ctx context.Context, customerRef string, delta int, reason, actorID string) (store.Customer, error) {
	if strings.TrimSpace(reason) == "" {
		return store.Customer{}, Err(CodeInvalidRequest, "Комментарий (reason) обязателен")
	}
	c, err := s.resolveCustomer(ctx, customerRef)
	if err != nil {
		return store.Customer{}, err
	}
	err = s.Store.InTx(ctx, func(tx *store.Tx) error {
		locked, e := tx.LockCustomer(ctx, c.ID)
		if e != nil {
			return e
		}
		next := locked.Points + delta
		if next < 0 {
			return Err(CodeInsufficientPoints, "Недостаточно баллов")
		}
		now := time.Now().UTC()
		if e := tx.InsertLedger(ctx, locked.ID, "", delta, store.ReasonAdjust+": "+reason, actorID, now); e != nil {
			return e
		}
		if e := tx.SetPoints(ctx, locked.ID, next, now); e != nil {
			return e
		}
		c.Points = next
		c.UpdatedAt = now
		return nil
	})
	if err != nil {
		return store.Customer{}, err
	}
	s.enqueue(ctx, c.ID)
	return c, nil
}

func (s *Service) enqueue(ctx context.Context, customerID string) {
	if s.Jobs == nil {
		return
	}
	s.Jobs.Enqueue(ctx, customerID)
}

func NormalizePhone(raw string) string {
	var digits strings.Builder
	for _, r := range raw {
		if unicode.IsDigit(r) {
			digits.WriteRune(r)
		}
	}
	d := digits.String()
	if d == "" {
		return ""
	}
	if len(d) == 11 && d[0] == '8' {
		d = "7" + d[1:]
	}
	if len(d) == 10 {
		d = "7" + d
	}
	return "+" + d
}
