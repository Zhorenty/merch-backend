package googlew

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"merch/backend/internal/config"
	"merch/backend/internal/store"
)

type Client struct {
	log     *slog.Logger
	cfg     config.Config
	sa      *google.Credentials
	http    *http.Client
	enabled bool
}

func New(cfg config.Config, log *slog.Logger) *Client {
	c := &Client{log: log, cfg: cfg, http: &http.Client{Timeout: 10 * time.Second}}
	if cfg.GoogleSAJSON == "" || cfg.GoogleIssuerID == "" {
		return c
	}
	raw, err := os.ReadFile(cfg.GoogleSAJSON)
	if err != nil {
		log.Warn("google sa json not loaded, using no-op adapter", "err", err)
		return c
	}
	creds, err := google.CredentialsFromJSON(context.Background(), raw, "https://www.googleapis.com/auth/wallet_object.issuer")
	if err != nil {
		log.Warn("google credentials parse failed, using no-op adapter", "err", err)
		return c
	}
	c.sa = creds
	c.enabled = true
	return c
}

func (c *Client) Enabled() bool { return c.enabled }

func (c *Client) SaveURL(cust store.Customer) (string, error) {
	if !c.enabled || c.sa == nil {
		return "", nil
	}
	var sa struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	raw, err := os.ReadFile(c.cfg.GoogleSAJSON)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(raw, &sa); err != nil {
		return "", err
	}
	obj := c.object(cust)
	claims := jwt.MapClaims{
		"iss": sa.ClientEmail,
		"aud": "google",
		"typ": "savetowallet",
		"iat": time.Now().Unix(),
		"payload": map[string]any{
			"loyaltyObjects": []any{obj},
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(sa.PrivateKey))
	if err != nil {
		return "", err
	}
	signed, err := tok.SignedString(key)
	if err != nil {
		return "", err
	}
	return "https://pay.google.com/gp/v/save/" + signed, nil
}

func (c *Client) PatchPoints(ctx context.Context, objectID string, points int) error {
	if !c.enabled || c.sa == nil {
		if c.log != nil {
			c.log.Info("wallet update skipped", "provider", "google")
		}
		return nil
	}
	ts := c.sa.TokenSource
	tok, err := ts.Token()
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{
		"loyaltyPoints": map[string]any{
			"label":   "Баллы",
			"balance": map[string]any{"int": points},
		},
	})
	url := "https://walletobjects.googleapis.com/walletobjects/v1/loyaltyobject/" + objectID + "?updateMask=loyaltyPoints"
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("google patch %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (c *Client) object(cust store.Customer) map[string]any {
	return map[string]any{
		"id":          cust.GoogleObjectID,
		"classId":     c.cfg.GoogleClassID(),
		"state":       "ACTIVE",
		"accountId":   cust.Barcode,
		"accountName": cust.DisplayName,
		"loyaltyPoints": map[string]any{
			"label":   "Баллы",
			"balance": map[string]any{"int": cust.Points},
		},
		"barcode": map[string]any{
			"type":  "QR_CODE",
			"value": cust.Barcode,
		},
	}
}

func TokenSource(ctx context.Context, jsonPath string) (oauth2.TokenSource, error) {
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}
	creds, err := google.CredentialsFromJSON(ctx, raw, "https://www.googleapis.com/auth/wallet_object.issuer")
	if err != nil {
		return nil, err
	}
	return creds.TokenSource, nil
}
