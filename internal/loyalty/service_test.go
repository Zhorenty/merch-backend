package loyalty_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"merch/backend/internal/auth"
	"merch/backend/internal/loyalty"
	"merch/backend/internal/store"
)

func testStore(t *testing.T) (*store.Store, *loyalty.Service, store.Staff) {
	t.Helper()
	ctx := context.Background()
	name := "sqlite:file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	st, err := store.Open(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	hash, err := auth.HashSecret("secret")
	if err != nil {
		t.Fatal(err)
	}
	staff := store.Staff{
		ID: uuid.NewString(), StoreID: store.DefaultStoreID(), Login: "c-" + t.Name(),
		Name: "Кассир", PasswordHash: hash, Role: auth.RoleShiftLead, Active: true, CreatedAt: time.Now().UTC(),
	}
	if err := st.CreateStaff(ctx, staff); err != nil {
		t.Fatal(err)
	}
	svc := &loyalty.Service{Store: st}
	return st, svc, staff
}

func enroll(t *testing.T, svc *loyalty.Service, name, phone string) store.Customer {
	t.Helper()
	res, err := svc.Enroll(context.Background(), loyalty.EnrollInput{Name: name, Phone: phone})
	if err != nil {
		t.Fatal(err)
	}
	return res.Customer
}

func TestCommitFormulaAndBalance(t *testing.T) {
	_, svc, staff := testStore(t)
	c := enroll(t, svc, "Анна", "")
	if c.Points != 0 {
		t.Fatalf("start points=%d", c.Points)
	}
	// give 500 points via adjust so redeem 500 is possible
	if _, err := svc.Adjust(context.Background(), c.Barcode, 500, "seed", staff.ID); err != nil {
		t.Fatal(err)
	}
	rid := uuid.NewString()
	res, err := svc.Commit(context.Background(), loyalty.CommitInput{
		ReceiptID: rid, Barcode: c.Barcode, AmountRub: 4500, RedeemPoints: 500,
		StoreID: staff.StoreID, StaffID: staff.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.EarnPoints != 200 {
		t.Fatalf("earn=%d want 200", res.EarnPoints)
	}
	// 0+500 -500 +200 = 200
	if res.Points != 200 {
		t.Fatalf("points=%d want 200", res.Points)
	}
}

func TestCommitIdempotent(t *testing.T) {
	st, svc, staff := testStore(t)
	c := enroll(t, svc, "Анна", "")
	_, _ = svc.Adjust(context.Background(), c.Barcode, 500, "seed", staff.ID)
	rid := uuid.NewString()
	in := loyalty.CommitInput{ReceiptID: rid, Barcode: c.Barcode, AmountRub: 4500, RedeemPoints: 500, StoreID: staff.StoreID, StaffID: staff.ID}
	a, err := svc.Commit(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Commit(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !b.IdempotentReplay {
		t.Fatal("expected replay")
	}
	if a.Points != b.Points || a.EarnPoints != b.EarnPoints {
		t.Fatalf("mismatch %+v %+v", a, b)
	}
	got, _ := st.GetCustomerByID(context.Background(), c.ID)
	if got.Points != a.Points {
		t.Fatalf("balance changed on replay: %d vs %d", got.Points, a.Points)
	}
}

func TestRedeemBelowMinDoesNotChangeBalance(t *testing.T) {
	st, svc, staff := testStore(t)
	c := enroll(t, svc, "Анна", "")
	_, _ = svc.Adjust(context.Background(), c.Barcode, 500, "seed", staff.ID)
	_, err := svc.Commit(context.Background(), loyalty.CommitInput{
		ReceiptID: uuid.NewString(), Barcode: c.Barcode, AmountRub: 4500, RedeemPoints: 50,
		StoreID: staff.StoreID, StaffID: staff.ID,
	})
	if !loyalty.Is(err, loyalty.CodeBelowMinRedeem) {
		t.Fatalf("err=%v", err)
	}
	got, _ := st.GetCustomerByID(context.Background(), c.ID)
	if got.Points != 500 {
		t.Fatalf("points=%d", got.Points)
	}
}

func TestRedeemExceedsShare(t *testing.T) {
	_, svc, staff := testStore(t)
	c := enroll(t, svc, "Анна", "")
	_, _ = svc.Adjust(context.Background(), c.Barcode, 800, "seed", staff.ID)
	_, err := svc.Commit(context.Background(), loyalty.CommitInput{
		ReceiptID: uuid.NewString(), Barcode: c.Barcode, AmountRub: 1000, RedeemPoints: 600,
		StoreID: staff.StoreID, StaffID: staff.ID,
	})
	if !loyalty.Is(err, loyalty.CodeExceedsShare) {
		t.Fatalf("err=%v", err)
	}
}

func TestBlockedCustomer(t *testing.T) {
	st, svc, staff := testStore(t)
	c := enroll(t, svc, "Анна", "")
	if err := st.SetCustomerBlocked(context.Background(), c.ID, true); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Lookup(context.Background(), c.Barcode)
	if !loyalty.Is(err, loyalty.CodeCustomerBlocked) {
		t.Fatalf("err=%v", err)
	}
	_, err = svc.Commit(context.Background(), loyalty.CommitInput{
		ReceiptID: uuid.NewString(), Barcode: c.Barcode, AmountRub: 1000, RedeemPoints: 0,
		StoreID: staff.StoreID, StaffID: staff.ID,
	})
	if !loyalty.Is(err, loyalty.CodeCustomerBlocked) {
		t.Fatalf("err=%v", err)
	}
}

func TestRefundIdempotent(t *testing.T) {
	_, svc, staff := testStore(t)
	c := enroll(t, svc, "Анна", "")
	_, _ = svc.Adjust(context.Background(), c.Barcode, 500, "seed", staff.ID)
	rid := uuid.NewString()
	_, err := svc.Commit(context.Background(), loyalty.CommitInput{
		ReceiptID: rid, Barcode: c.Barcode, AmountRub: 4500, RedeemPoints: 500,
		StoreID: staff.StoreID, StaffID: staff.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	a, err := svc.Refund(context.Background(), rid, staff.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Refund(context.Background(), rid, staff.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !b.IdempotentReplay || a.Points != b.Points {
		t.Fatalf("refund replay %+v %+v", a, b)
	}
	// after refund: reverse earn 200, return 500 → 200-200+500 = 500
	if a.Points != 500 {
		t.Fatalf("points after refund=%d want 500", a.Points)
	}
}

func TestEnrollSamePhoneOrCookie(t *testing.T) {
	_, svc, _ := testStore(t)
	a, err := svc.Enroll(context.Background(), loyalty.EnrollInput{Name: "Анна", Phone: "+79001112233"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Enroll(context.Background(), loyalty.EnrollInput{Name: "Другая", Phone: "8 900 111 22 33"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Created == false || b.Created || a.Customer.ID != b.Customer.ID {
		t.Fatalf("phone enroll duplicate: a=%+v b=%+v", a, b)
	}
	c, err := svc.Enroll(context.Background(), loyalty.EnrollInput{Name: "X", CookieID: a.Customer.ID})
	if err != nil {
		t.Fatal(err)
	}
	if c.Created || c.Customer.ID != a.Customer.ID {
		t.Fatal("cookie enroll created a second customer")
	}
}
