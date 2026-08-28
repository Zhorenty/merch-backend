package barcode

import "testing"

func TestNewAndValid(t *testing.T) {
	s, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if !Valid(s) {
		t.Fatalf("invalid generated %q", s)
	}
	if !Valid("MCH-7K2P9Q4R") {
		t.Fatal("example from TZ should be valid")
	}
	if Valid("MCH-ILOU1234") { // I L O U forbidden
		t.Fatal("crockford forbids I L O U")
	}
}
