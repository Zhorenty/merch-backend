package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config is loaded from the process environment. Secrets stay out of the repo.
type Config struct {
	HTTPAddr         string        `env:"HTTP_ADDR" envDefault:":8080"`
	APIBaseURL       string        `env:"API_BASE_URL" envDefault:"http://localhost:8080"`
	PublicBaseURL    string        `env:"PUBLIC_BASE_URL" envDefault:"http://localhost:8080"`
	CORSOrigins      string        `env:"CORS_ORIGINS"`
	DatabaseURL      string        `env:"DATABASE_URL" envDefault:"sqlite:./data/merch.db"`
	CashierJWTSecret string        `env:"CASHIER_JWT_SECRET" envDefault:"dev-cashier-secret-change-me"`
	AdminJWTSecret   string        `env:"ADMIN_JWT_SECRET" envDefault:"dev-admin-secret-change-me"`
	CashierJWTTTL    time.Duration `env:"CASHIER_JWT_TTL" envDefault:"12h"`
	AdminJWTTTL      time.Duration `env:"ADMIN_JWT_TTL" envDefault:"12h"`

	AdminBootstrapLogin    string `env:"ADMIN_BOOTSTRAP_LOGIN" envDefault:"admin"`
	AdminBootstrapPassword string `env:"ADMIN_BOOTSTRAP_PASSWORD" envDefault:"changeme"`
	AdminBootstrapName     string `env:"ADMIN_BOOTSTRAP_NAME" envDefault:"Админ"`

	CashierBootstrapLogin    string `env:"CASHIER_BOOTSTRAP_LOGIN"`
	CashierBootstrapPassword string `env:"CASHIER_BOOTSTRAP_PASSWORD"`
	CashierBootstrapName     string `env:"CASHIER_BOOTSTRAP_NAME" envDefault:"Кассир"`

	// Empty TERMS_URL becomes {PUBLIC_BASE_URL}/loyalty-terms.
	TermsURL        string `env:"TERMS_URL"`
	SupportContact  string `env:"SUPPORT_CONTACT" envDefault:"Telegram @merch"`
	AppMinSupported string `env:"APP_MIN_SUPPORTED" envDefault:"1.0.0"`
	AppDownloadURL  string `env:"APP_DOWNLOAD_URL" envDefault:"https://example.com/merch-kassa.apk"`

	AppleTeamID           string `env:"APPLE_TEAM_ID"`
	ApplePassTypeID       string `env:"APPLE_PASS_TYPE_ID" envDefault:"pass.com.merch.loyalty"`
	ApplePassCertP12      string `env:"APPLE_PASS_CERT_P12"`
	ApplePassCertPassword string `env:"APPLE_PASS_CERT_PASSWORD"`
	AppleWWDRCert         string `env:"APPLE_WWDR_CERT"`
	AppleAPNsP8           string `env:"APPLE_APNS_P8"`
	AppleAPNsKeyID        string `env:"APPLE_APNS_KEY_ID"`
	AppleAPNsProduction   bool   `env:"APPLE_APNS_PRODUCTION" envDefault:"false"`

	GoogleIssuerID    string `env:"GOOGLE_ISSUER_ID"`
	GoogleSAJSON      string `env:"GOOGLE_SA_JSON"`
	GoogleClassSuffix string `env:"GOOGLE_CLASS_SUFFIX" envDefault:"merch_loyalty"`

	PassBGColor    string `env:"PASS_BG_COLOR" envDefault:"rgb(255,255,255)"`
	PassFGColor    string `env:"PASS_FG_COLOR" envDefault:"rgb(28,29,77)"`
	PassLabelColor string `env:"PASS_LABEL_COLOR" envDefault:"rgb(110,112,150)"`

	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`
}

func Load() (Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	c.APIBaseURL = strings.TrimRight(c.APIBaseURL, "/")
	c.PublicBaseURL = strings.TrimRight(c.PublicBaseURL, "/")
	terms := strings.TrimSpace(c.TermsURL)
	if terms == "" || terms == "https://example.com/loyalty-terms" {
		c.TermsURL = c.PublicBaseURL + "/loyalty-terms"
	}
	return c, nil
}

func (c Config) CORSList() []string {
	if strings.TrimSpace(c.CORSOrigins) == "" {
		return []string{c.PublicBaseURL, c.APIBaseURL}
	}
	parts := strings.Split(c.CORSOrigins, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, strings.TrimRight(p, "/"))
		}
	}
	return out
}

func (c Config) GoogleClassID() string {
	if c.GoogleIssuerID == "" {
		return "merch.merch_loyalty"
	}
	return c.GoogleIssuerID + "." + c.GoogleClassSuffix
}

func (c Config) GoogleObjectID(customerID string) string {
	issuer := c.GoogleIssuerID
	if issuer == "" {
		issuer = "merch"
	}
	return issuer + "." + customerID
}
