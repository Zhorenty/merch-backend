package barcode

import (
	"crypto/rand"
	"math/big"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// Prefix and alphabet are fixed for MERCH. Crockford Base32 without I, L, O, U.
const Prefix = "MCH-"
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var re = regexp.MustCompile(`^MCH-[0-9A-HJKMNP-TV-Z]{8}$`)

func New() (string, error) {
	b := make([]byte, 8)
	max := big.NewInt(int64(len(crockford)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = crockford[n.Int64()]
	}
	return Prefix + string(b), nil
}

func Valid(s string) bool {
	s = strings.TrimSpace(strings.ToUpper(s))
	return re.MatchString(s)
}

func Normalize(s string) string {
	return strings.TrimSpace(strings.ToUpper(s))
}

func LooksLikeCustomerID(s string) bool {
	_, err := uuid.Parse(strings.TrimSpace(s))
	return err == nil
}
