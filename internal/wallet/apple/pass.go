package apple

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"go.mozilla.org/pkcs7"
	"golang.org/x/crypto/pkcs12"
	"merch/backend/internal/config"
	"merch/backend/internal/store"
)

type Client struct {
	Log     *slog.Logger
	Cfg     config.Config
	Signer  Signer
	Pusher  Pusher
	enabled bool
}

type Signer interface {
	Sign(manifestJSON []byte) ([]byte, error)
}

type Pusher interface {
	Push(ctx context.Context, tokens []string) error
}

func New(cfg config.Config, log *slog.Logger) *Client {
	c := &Client{Log: log, Cfg: cfg}
	if cfg.ApplePassCertP12 != "" {
		if s, err := LoadPKCS12Signer(cfg.ApplePassCertP12, cfg.ApplePassCertPassword, cfg.AppleWWDRCert); err != nil {
			log.Warn("apple pass cert not loaded, using stub signer", "err", err)
			c.Signer = StubSigner{}
		} else {
			c.Signer = s
			c.enabled = true
		}
	} else {
		c.Signer = StubSigner{}
	}
	if cfg.AppleAPNsP8 != "" && cfg.AppleTeamID != "" && cfg.AppleAPNsKeyID != "" {
		c.Pusher = NewAPNs(cfg, log)
		c.enabled = true
	} else {
		c.Pusher = noopPusher{log: log}
	}
	return c
}

func (c *Client) Enabled() bool { return c.enabled }

func (c *Client) PushUpdate(ctx context.Context, tokens []string) error {
	if c.Pusher == nil {
		return nil
	}
	return c.Pusher.Push(ctx, tokens)
}

func (c *Client) BuildPKPass(ctx context.Context, cust store.Customer, _ string) ([]byte, error) {
	_ = ctx
	pass := c.passJSON(cust)
	files := map[string][]byte{
		"pass.json": pass,
	}
	icon := solidPNG(29, 29, parseRGB(c.Cfg.PassBGColor, color.RGBA{26, 26, 26, 255}))
	logo := solidPNG(160, 50, parseRGB(c.Cfg.PassBGColor, color.RGBA{26, 26, 26, 255}))
	strip := solidPNG(375, 144, parseRGB(c.Cfg.PassBGColor, color.RGBA{26, 26, 26, 255}))
	files["icon.png"] = icon
	files["icon@2x.png"] = solidPNG(58, 58, parseRGB(c.Cfg.PassBGColor, color.RGBA{26, 26, 26, 255}))
	files["icon@3x.png"] = solidPNG(87, 87, parseRGB(c.Cfg.PassBGColor, color.RGBA{26, 26, 26, 255}))
	files["logo.png"] = logo
	files["logo@2x.png"] = solidPNG(320, 100, parseRGB(c.Cfg.PassBGColor, color.RGBA{26, 26, 26, 255}))
	files["strip.png"] = strip
	files["strip@2x.png"] = solidPNG(750, 288, parseRGB(c.Cfg.PassBGColor, color.RGBA{26, 26, 26, 255}))

	manifest := map[string]string{}
	for name, data := range files {
		sum := sha1.Sum(data)
		manifest[name] = hex.EncodeToString(sum[:])
	}
	manJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	files["manifest.json"] = manJSON
	sig, err := c.Signer.Sign(manJSON)
	if err != nil {
		return nil, err
	}
	files["signature"] = sig

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *Client) passJSON(cust store.Customer) []byte {
	web := strings.TrimRight(c.Cfg.APIBaseURL, "/") + "/passes/"
	back := fmt.Sprintf("Баллы начисляются с покупок в MERCH и списываются на кассе. Карта — не платёжное средство. Правила может изменить магазин. Вопросы: %s.", c.Cfg.SupportContact)
	doc := map[string]any{
		"formatVersion":       1,
		"passTypeIdentifier":  c.Cfg.ApplePassTypeID,
		"teamIdentifier":      c.Cfg.AppleTeamID,
		"serialNumber":        cust.ID,
		"organizationName":    "MERCH",
		"description":         "Карта лояльности MERCH",
		"logoText":            "MERCH",
		"foregroundColor":     c.Cfg.PassFGColor,
		"backgroundColor":     c.Cfg.PassBGColor,
		"labelColor":          c.Cfg.PassLabelColor,
		"webServiceURL":       web,
		"authenticationToken": cust.AppleAuthToken,
		"barcodes": []map[string]any{{
			"format":          "PKBarcodeFormatQR",
			"message":         cust.Barcode,
			"messageEncoding": "iso-8859-1",
			"altText":         cust.Barcode,
		}},
		"storeCard": map[string]any{
			"headerFields": []map[string]any{{
				"key":   "card",
				"label": "Карта",
				"value": cust.Barcode,
			}},
			"primaryFields": []map[string]any{{
				"key":           "points",
				"label":         "Баллы",
				"value":         strconv.Itoa(cust.Points),
				"changeMessage": "Баланс: %@",
			}},
			"secondaryFields": func() []map[string]any {
				if strings.TrimSpace(cust.DisplayName) == "" {
					return []map[string]any{}
				}
				return []map[string]any{{
					"key":   "name",
					"label": "Имя",
					"value": cust.DisplayName,
				}}
			}(),
			"backFields": []map[string]any{
				{"key": "rules", "label": "Правила", "value": back},
				{"key": "terms", "label": "Полные правила", "value": c.Cfg.TermsURL},
				{"key": "support", "label": "Контакты", "value": c.Cfg.SupportContact},
			},
		},
	}
	b, _ := json.Marshal(doc)
	return b
}

type StubSigner struct{}

func (StubSigner) Sign(manifestJSON []byte) ([]byte, error) {
	// Placeholder CMS blob so the zip is a well-formed .pkpass without live certs.
	sum := sha1.Sum(manifestJSON)
	return []byte("STUB-SIGNATURE-" + hex.EncodeToString(sum[:])), nil
}

type PKCS12Signer struct {
	cert *x509.Certificate
	key  crypto.PrivateKey
	wwdr *x509.Certificate
}

func LoadPKCS12Signer(p12Path, password, wwdrPath string) (*PKCS12Signer, error) {
	raw, err := os.ReadFile(p12Path)
	if err != nil {
		return nil, err
	}
	key, cert, err := pkcs12.Decode(raw, password)
	if err != nil {
		return nil, err
	}
	priv, ok := key.(crypto.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("p12: private key type %T", key)
	}
	s := &PKCS12Signer{cert: cert, key: priv}
	if wwdrPath != "" {
		wb, err := os.ReadFile(wwdrPath)
		if err != nil {
			return nil, err
		}
		block, _ := pem.Decode(wb)
		if block == nil {
			certs, err := x509.ParseCertificates(wb)
			if err != nil || len(certs) == 0 {
				return nil, fmt.Errorf("parse WWDR")
			}
			s.wwdr = certs[0]
		} else {
			c, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, err
			}
			s.wwdr = c
		}
	}
	return s, nil
}

func (s *PKCS12Signer) Sign(manifestJSON []byte) ([]byte, error) {
	if s == nil || s.cert == nil || s.key == nil {
		return StubSigner{}.Sign(manifestJSON)
	}
	toBeSigned, err := pkcs7.NewSignedData(manifestJSON)
	if err != nil {
		return nil, err
	}
	if err := toBeSigned.AddSigner(s.cert, s.key, pkcs7.SignerInfoConfig{}); err != nil {
		return nil, err
	}
	if s.wwdr != nil {
		toBeSigned.AddCertificate(s.wwdr)
	}
	toBeSigned.Detach()
	return toBeSigned.Finish()
}

type noopPusher struct{ log *slog.Logger }

func (n noopPusher) Push(ctx context.Context, tokens []string) error {
	_ = ctx
	if n.log != nil {
		n.log.Info("wallet update skipped", "provider", "apns", "tokens", len(tokens))
	}
	return nil
}

func solidPNG(w, h int, col color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, col)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func parseRGB(s string, def color.RGBA) color.RGBA {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimPrefix(s, "rgb(")
	s = strings.TrimSuffix(s, ")")
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return def
	}
	var v [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return def
		}
		v[i] = n
	}
	return color.RGBA{uint8(v[0]), uint8(v[1]), uint8(v[2]), 255}
}

func NowRFC1123() string { return time.Now().UTC().Format(time.RFC1123) }
