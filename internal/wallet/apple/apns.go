package apple

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"merch/backend/internal/config"
)

// APNs sends a silent pass update (empty body) per PassKit §6.3.
type APNs struct {
	log    *slog.Logger
	cfg    config.Config
	key    *ecdsa.PrivateKey
	client *http.Client
}

func NewAPNs(cfg config.Config, log *slog.Logger) *APNs {
	a := &APNs{
		log: log,
		cfg: cfg,
		client: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
	if raw, err := os.ReadFile(cfg.AppleAPNsP8); err == nil {
		if k, err := parseP8(raw); err == nil {
			a.key = k
		} else if log != nil {
			log.Warn("apns p8 parse failed", "err", err)
		}
	}
	return a
}

func parseP8(pemBytes []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no pem block")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ec, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not ecdsa")
	}
	return ec, nil
}

func (a *APNs) Push(ctx context.Context, tokens []string) error {
	if a.key == nil {
		if a.log != nil {
			a.log.Info("wallet update skipped", "provider", "apns")
		}
		return nil
	}
	host := "https://api.sandbox.push.apple.com"
	if a.cfg.AppleAPNsProduction {
		host = "https://api.push.apple.com"
	}
	tok, err := a.bearer()
	if err != nil {
		return err
	}
	for _, device := range tokens {
		url := host + "/3/device/" + device
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte("{}")))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "bearer "+tok)
		req.Header.Set("apns-topic", a.cfg.ApplePassTypeID)
		req.Header.Set("apns-push-type", "background")
		req.Header.Set("apns-priority", "5")
		resp, err := a.client.Do(req)
		if err != nil {
			if a.log != nil {
				a.log.Error("apns push failed", "err", err)
			}
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 300 && a.log != nil {
			a.log.Warn("apns status", "code", resp.StatusCode)
		}
	}
	return nil
}

func (a *APNs) bearer() (string, error) {
	claims := jwt.MapClaims{
		"iss": a.cfg.AppleTeamID,
		"iat": time.Now().Unix(),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	t.Header["kid"] = a.cfg.AppleAPNsKeyID
	return t.SignedString(a.key)
}

func MaskToken(s string) string {
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + strings.Repeat("*", 8)
}
