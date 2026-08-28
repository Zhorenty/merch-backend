package loyalty

import "testing"

func TestComputeEarn_Table(t *testing.T) {
	s := DefaultSettings()
	cases := []struct {
		name   string
		amount int
		redeem int
		want   int
	}{
		{"tz example 4500/500/5%", 4500, 500, 200},
		{"1990 round down", 1990, 0, 99},
		{"no redeem", 1000, 0, 50},
		{"full share 50%", 1000, 500, 25},
		{"zero amount", 0, 0, 0},
		{"redeem equals amount", 500, 500, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeEarn(tc.amount, tc.redeem, s)
			if got != tc.want {
				t.Fatalf("earn=%d want=%d", got, tc.want)
			}
		})
	}
}

func TestCheckRedeem_Table(t *testing.T) {
	s := DefaultSettings()
	cases := []struct {
		name      string
		points    int
		amount    int
		requested int
		code      string
	}{
		{"ok 500 of 4500 with 500 pts", 500, 4500, 500, ""},
		{"below min 50", 500, 4500, 50, CodeBelowMinRedeem},
		{"over 50% of 1000", 800, 1000, 600, CodeExceedsShare},
		{"insufficient", 100, 4500, 200, CodeInsufficientPoints},
		{"zero always ok", 0, 4500, 0, ""},
		{"exact 50%", 1000, 1000, 500, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckRedeem(tc.points, tc.amount, tc.requested, s)
			if tc.code == "" {
				if err != nil {
					t.Fatalf("unexpected %v", err)
				}
				return
			}
			if err == nil || err.Code != tc.code {
				t.Fatalf("got %#v want %s", err, tc.code)
			}
		})
	}
}

func TestQuoteRedeem_DeniedDoesNotProposeEarn(t *testing.T) {
	q := QuoteRedeem(500, 4500, 50, DefaultSettings())
	if q.Allowed {
		t.Fatal("expected denied")
	}
	if q.Code != CodeBelowMinRedeem {
		t.Fatalf("code=%s", q.Code)
	}
	if q.EarnPoints != 0 || q.RedeemPoints != 0 {
		t.Fatalf("denied quote must not apply redeem/earn: %+v", q)
	}
}
