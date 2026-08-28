package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	RoleCashier   = "cashier"
	RoleShiftLead = "shift_lead"
	RoleAdmin     = "admin"
	KindCashier   = "cashier"
	KindAdmin     = "admin"
)

type Claims struct {
	jwt.RegisteredClaims
	Role    string `json:"role"`
	StoreID string `json:"store_id"`
	Kind    string `json:"kind"`
	Login   string `json:"login"`
}

func HashSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func CheckSecret(hash, plain string) bool {
	if hash == "" || plain == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

func NewJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func Sign(secret, kind, staffID, login, role, storeID, jti string, ttl time.Duration) (string, time.Time, error) {
	if secret == "" {
		return "", time.Time{}, fmt.Errorf("jwt secret is empty")
	}
	exp := time.Now().Add(ttl)
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   staffID,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
		Role:    role,
		StoreID: storeID,
		Kind:    kind,
		Login:   login,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	return s, exp, err
}

func Parse(secret, token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

func CanRefund(role string) bool {
	return role == RoleShiftLead || role == RoleAdmin
}

func IsAdmin(role string) bool {
	return role == RoleAdmin
}

func ValidRole(role string) bool {
	switch role {
	case RoleCashier, RoleShiftLead, RoleAdmin:
		return true
	default:
		return false
	}
}

func RandomToken(nBytes int) (string, error) {
	if nBytes < 16 {
		nBytes = 16
	}
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
