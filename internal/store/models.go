package store

import (
	"database/sql"
	"time"
)

type StoreRow struct {
	ID        string
	Name      string
	Address   string
	CreatedAt time.Time
}

type Staff struct {
	ID           string
	StoreID      string
	Login        string
	Name         string
	PasswordHash string
	PINHash      string
	Role         string
	Active       bool
	CreatedAt    time.Time
}

type Customer struct {
	ID             string
	Barcode        string
	DisplayName    string
	Phone          string
	AppleAuthToken string
	GoogleObjectID string
	Points         int
	Blocked        bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Receipt struct {
	ID                string
	StoreID           string
	StaffID           string
	CustomerID        string
	AmountRub         int
	RedeemPoints      int
	EarnPoints        int
	Status            string
	PointsAfter       int
	PointsAfterRefund sql.NullInt64
	CreatedAt         time.Time
	RefundedAt        sql.NullTime
}

type LedgerEntry struct {
	ID           string
	CustomerID   string
	ReceiptID    sql.NullString
	Delta        int
	Reason       string
	ActorStaffID sql.NullString
	CreatedAt    time.Time
}

type AppleDevice struct {
	ID              string
	CustomerID      string
	DeviceLibraryID string
	PushToken       string
	CreatedAt       time.Time
}

type WalletJob struct {
	ID          string
	CustomerID  string
	Kind        string
	Attempts    int
	LastError   sql.NullString
	CreatedAt   time.Time
	ProcessedAt sql.NullTime
}

const (
	ReceiptCommitted   = "committed"
	ReceiptRefunded    = "refunded"
	ReasonEarn         = "earn"
	ReasonRedeem       = "redeem"
	ReasonRefundEarn   = "refund_earn"
	ReasonRefundRedeem = "refund_redeem"
	ReasonAdjust       = "adjust"
)
