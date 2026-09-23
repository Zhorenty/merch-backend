package apple

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image/png"
	"log/slog"
	"strings"
	"testing"

	"merch/backend/internal/config"
	"merch/backend/internal/store"
)

func TestPassShowsLogoAndBalanceInHeader(t *testing.T) {
	c := New(config.Config{
		APIBaseURL:      "https://api.example",
		ApplePassTypeID: "pass.com.merch.loyalty",
		AppleTeamID:     "TEAM",
		PassBGColor:     "rgb(255,255,255)",
		PassFGColor:     "rgb(28,29,77)",
		PassLabelColor:  "rgb(110,112,150)",
		SupportContact:  "help@example",
		TermsURL:        "https://example/terms",
	}, slog.Default())
	cust := store.Customer{
		ID:             "c1",
		Barcode:        "MCH-ABCDEFGH",
		Points:         42,
		DisplayName:    "Анна",
		AppleAuthToken: "tok",
	}

	var doc map[string]any
	back := "1 балл = 1 ₽.\nТочки:\nMERCH — ул. Пример, 1"
	if err := json.Unmarshal(c.passJSON(cust, 5, back), &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["logoText"]; ok {
		t.Fatal("logoText sits beside the wordmark and covers the balance in the stack")
	}
	card := doc["storeCard"].(map[string]any)
	header := card["headerFields"].([]any)
	if len(header) != 1 {
		t.Fatalf("header fields: %d", len(header))
	}
	field := header[0].(map[string]any)
	if field["label"] != "Баланс" || field["value"] != "42" {
		t.Fatalf("header: %#v", field)
	}
	if len(card["primaryFields"].([]any)) != 0 {
		t.Fatal("primary field would hide the balance until the pass is opened")
	}
	secondary := card["secondaryFields"].([]any)
	if len(secondary) != 2 {
		t.Fatalf("secondary fields: %d", len(secondary))
	}
	owner := secondary[0].(map[string]any)
	bonus := secondary[1].(map[string]any)
	if owner["label"] != "Владелец сваги" || owner["value"] != "Анна" {
		t.Fatalf("owner: %#v", owner)
	}
	if bonus["label"] != "Бонус" || bonus["value"] != "5%" {
		t.Fatalf("bonus: %#v", bonus)
	}
	backs := card["backFields"].([]any)
	terms := backs[0].(map[string]any)
	if terms["label"] != "Условия программы лояльности" || !strings.Contains(terms["value"].(string), "Точки:") {
		t.Fatalf("back: %#v", terms)
	}

	raw, err := c.BuildPKPass(t.Context(), cust, 5, back)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]int{
		"logo.png":     {160, 50},
		"icon.png":     {29, 29},
		"strip.png":    {375, 144},
		"strip@2x.png": {750, 288},
		"strip@3x.png": {1125, 432},
	}
	for _, f := range zr.File {
		box, ok := want[f.Name]
		if !ok {
			continue
		}
		delete(want, f.Name)
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		b := img.Bounds()
		if f.Name == "logo.png" {
			if b.Dx() > box[0] || b.Dy() > box[1] {
				t.Fatalf("logo %dx%d exceeds the header row", b.Dx(), b.Dy())
			}
			continue
		}
		if b.Dx() != box[0] || b.Dy() != box[1] {
			t.Fatalf("%s is %dx%d, want %dx%d", f.Name, b.Dx(), b.Dy(), box[0], box[1])
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing images: %v", want)
	}
}
