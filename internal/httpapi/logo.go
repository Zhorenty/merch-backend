package httpapi

import (
	_ "embed"
	"net/http"
)

// Square wordmark for the Google Wallet loyalty class program logo.
// Google fetches this URL itself; the pass artwork stays in the Apple assets.
//
//go:embed assets/wallet-logo.png
var walletLogo []byte

func (s *Server) getWalletLogo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(walletLogo)
}
