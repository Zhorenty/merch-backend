package loyalty

import (
	"strings"
	"testing"

	"merch/backend/internal/store"
)

func TestProgramDescriptionUsesSettingsAndStores(t *testing.T) {
	st := DefaultSettings()
	days := 30
	st.ExpireDays = &days
	st.EarnMinReceipt = 200
	text := ProgramDescription(st, []store.StoreRow{
		{Name: "ТЦ Европейский", Address: "пл. Киевского Вокзала, 2"},
		{Name: "Онлайн"},
	}, "Telegram @merch")

	for _, part := range []string{
		"1 балл = 1 ₽",
		"начисляется 5%",
		"5 баллов",
		"от 100 баллов",
		"50%",
		"чека 200 ₽",
		"через 30 дней",
		"ТЦ Европейский — пл. Киевского Вокзала, 2",
		"Онлайн",
		"Telegram @merch",
		"не платёжное средство",
	} {
		if !strings.Contains(text, part) {
			t.Fatalf("missing %q in %s", part, text)
		}
	}
}

func TestProgramDescriptionNoExpiryAndNoStores(t *testing.T) {
	text := ProgramDescription(DefaultSettings(), nil, "")
	if !strings.Contains(text, "Баллы не сгорают.") {
		t.Fatal(text)
	}
	if !strings.Contains(text, "Точки: список появится") {
		t.Fatal(text)
	}
}
