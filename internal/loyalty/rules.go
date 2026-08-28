package loyalty

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	CodeInsufficientPoints = "INSUFFICIENT_POINTS"
	CodeBelowMinRedeem     = "BELOW_MIN_REDEEM"
	CodeExceedsShare       = "EXCEEDS_RECEIPT_SHARE"
	CodeCustomerNotFound   = "CUSTOMER_NOT_FOUND"
	CodeCustomerBlocked    = "CUSTOMER_BLOCKED"
	CodeDuplicateReceipt   = "DUPLICATE_RECEIPT"
	CodeStaffForbidden     = "STAFF_FORBIDDEN"
	CodeInvalidRequest     = "INVALID_REQUEST"
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeInternal           = "INTERNAL"
)

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func Err(code, msg string) *Error {
	return &Error{Code: code, Message: msg}
}

func Is(err error, code string) bool {
	e, ok := err.(*Error)
	return ok && e.Code == code
}

type Settings struct {
	EarnPercent    int
	EarnRound      string
	EarnBase       string
	EarnMinReceipt int
	RedeemRate     int
	RedeemMin      int
	RedeemMaxShare int
	ExpireDays     *int
}

func DefaultSettings() Settings {
	return Settings{
		EarnPercent:    5,
		EarnRound:      "down",
		EarnBase:       "after_store_discount_before_points",
		EarnMinReceipt: 0,
		RedeemRate:     1,
		RedeemMin:      100,
		RedeemMaxShare: 50,
		ExpireDays:     nil,
	}
}

func ParseSettings(m map[string]string) Settings {
	st := DefaultSettings()
	if v, ok := m["earn_percent"]; ok {
		st.EarnPercent = atoi(v, st.EarnPercent)
	}
	if v, ok := m["earn_round"]; ok && v != "" {
		st.EarnRound = v
	}
	if v, ok := m["earn_base"]; ok && v != "" {
		st.EarnBase = v
	}
	if v, ok := m["earn_min_receipt"]; ok {
		st.EarnMinReceipt = atoi(v, st.EarnMinReceipt)
	}
	if v, ok := m["redeem_rate"]; ok {
		st.RedeemRate = atoi(v, st.RedeemRate)
	}
	if v, ok := m["redeem_min"]; ok {
		st.RedeemMin = atoi(v, st.RedeemMin)
	}
	if v, ok := m["redeem_max_share"]; ok {
		st.RedeemMaxShare = atoi(v, st.RedeemMaxShare)
	}
	if v, ok := m["expire_days"]; ok {
		v = strings.TrimSpace(v)
		if v == "" || v == "null" {
			st.ExpireDays = nil
		} else {
			n := atoi(v, 0)
			st.ExpireDays = &n
		}
	}
	return st
}

func atoi(s string, def int) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return v
}

func (s Settings) withDefaults() Settings {
	if s.RedeemRate <= 0 {
		s.RedeemRate = 1
	}
	if s.EarnPercent < 0 {
		s.EarnPercent = 0
	}
	if s.RedeemMaxShare < 0 {
		s.RedeemMaxShare = 0
	}
	if s.RedeemMaxShare > 100 {
		s.RedeemMaxShare = 100
	}
	if s.EarnRound == "" {
		s.EarnRound = "down"
	}
	return s
}

// ComputeEarn implements §3.2:
// points_earn = floor( (receipt_payable_rub - points_redeemed_as_rub) * earn_percent / 100 )
func ComputeEarn(amountRub, redeemPoints int, s Settings) int {
	s = s.withDefaults()
	if amountRub < s.EarnMinReceipt {
		return 0
	}
	redeemRub := redeemPoints * s.RedeemRate
	base := amountRub - redeemRub
	if base < 0 {
		base = 0
	}
	// integer division floors for non-negative values
	return base * s.EarnPercent / 100
}

func MaxRedeemable(points, amountRub int, s Settings) int {
	s = s.withDefaults()
	if points <= 0 || amountRub <= 0 {
		return 0
	}
	maxByShare := (amountRub * s.RedeemMaxShare / 100) / s.RedeemRate
	max := points
	if maxByShare < max {
		max = maxByShare
	}
	if max < s.RedeemMin {
		return 0
	}
	return max
}

func CheckRedeem(points, amountRub, requested int, s Settings) *Error {
	s = s.withDefaults()
	if requested == 0 {
		return nil
	}
	if requested < 0 {
		return Err(CodeInvalidRequest, "Количество баллов не может быть отрицательным")
	}
	if requested < s.RedeemMin {
		return Err(CodeBelowMinRedeem, fmt.Sprintf("Минимум для списания — %d баллов", s.RedeemMin))
	}
	if requested > points {
		return Err(CodeInsufficientPoints, "Недостаточно баллов")
	}
	maxRub := amountRub * s.RedeemMaxShare / 100
	requestedRub := requested * s.RedeemRate
	if requestedRub > maxRub {
		return Err(CodeExceedsShare, fmt.Sprintf("Нельзя списать больше %d%% суммы чека", s.RedeemMaxShare))
	}
	return nil
}

type Quote struct {
	Allowed         bool   `json:"allowed"`
	Code            string `json:"code,omitempty"`
	Reason          string `json:"reason,omitempty"`
	RequestedPoints int    `json:"requested_points"`
	MaxPoints       int    `json:"max_points"`
	RedeemPoints    int    `json:"redeem_points"`
	RedeemRub       int    `json:"redeem_rub"`
	EarnPoints      int    `json:"earn_points"`
	PayableRub      int    `json:"payable_rub"`
	CurrentPoints   int    `json:"current_points"`
}

func QuoteRedeem(points, amountRub, requested int, s Settings) Quote {
	s = s.withDefaults()
	q := Quote{
		RequestedPoints: requested,
		MaxPoints:       MaxRedeemable(points, amountRub, s),
		CurrentPoints:   points,
	}
	if err := CheckRedeem(points, amountRub, requested, s); err != nil {
		q.Allowed = false
		q.Code = err.Code
		q.Reason = err.Message
		return q
	}
	q.Allowed = true
	q.RedeemPoints = requested
	q.RedeemRub = requested * s.RedeemRate
	q.EarnPoints = ComputeEarn(amountRub, requested, s)
	q.PayableRub = amountRub - q.RedeemRub
	if q.PayableRub < 0 {
		q.PayableRub = 0
	}
	return q
}
